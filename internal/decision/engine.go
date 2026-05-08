// decision/engine.go
package decision

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wancm/trader-bot/internal/broker"
)

const retryDelay = time.Second

// Engine 是决策引擎的外观，协调信号过滤、上下文构建、AI 调用、下单等
type Engine struct {
	signalFilter  *SignalFilter
	mt5Client     *MT5Client
	ctxBuilder    *ContextBuilder
	aiClient      *AIClient
	postProcessor *PostProcessor
	broker        broker.Broker // 新增
	getPortfolio  func(symbol string) (PortfolioState, error)
	lastTick      map[string]TickData
	mu            sync.Mutex
	signalChan    chan SignalEvent
	logger        *slog.Logger
	aiLogRepo     *AILogRepository
}

// NewEngine 创建一个新的决策引擎
func NewEngine(
	rules []Rule,
	signalChan chan SignalEvent,
	mt5BaseURL, aiAPIKey, aiBaseURL, aiModel string,
	allowShort bool, minConfidence float64,
	logger *slog.Logger,
	dbPool *pgxpool.Pool,
	brk broker.Broker, // 新增
) *Engine {
	client := NewMT5Client(mt5BaseURL, logger)
	return &Engine{
		signalFilter:  NewSignalFilter(rules, signalChan, logger),
		mt5Client:     client,
		ctxBuilder:    NewContextBuilder(client),
		aiClient:      NewAIClient(aiAPIKey, aiBaseURL, aiModel),
		postProcessor: NewPostProcessor(allowShort, minConfidence),
		broker:        brk,
		lastTick:      make(map[string]TickData),
		signalChan:    signalChan,
		logger:        logger,
		aiLogRepo:     NewAILogRepository(dbPool),
	}
}

// ProcessTick 接收行情 tick，更新报价与缓存，触发限价单扫描和信号过滤
func (e *Engine) ProcessTick(ctx context.Context, tick TickData) {
	e.mu.Lock()
	e.lastTick[tick.Symbol] = tick
	e.mu.Unlock()

	// // 如果 broker 支持限价单扫描，则在每个 tick 时尝试触发挂单
	// if checker, ok := e.broker.(interface {
	// 	CheckPendingOrders(ctx context.Context) error
	// }); ok {
	// 	// 使用一个极短的超时，避免阻塞行情线程
	// 	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	// 	if err := checker.CheckPendingOrders(ctx); err != nil {
	// 		e.logger.Warn("pending order check failed", "err", err)
	// 	}
	// 	cancel()
	// }

	e.signalFilter.ProcessTick(tick)
}

// Start 启动信号事件消费循环
func (e *Engine) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-e.signalChan:
				if !ok {
					return
				}
				e.handleSignal(ctx, event)
			}
		}
	}()
}

// handleSignal 处理一次信号事件：构建上下文、调用 AI、后处理、下单
func (e *Engine) handleSignal(ctx context.Context, event SignalEvent) {
	e.mu.Lock()
	tick, ok := e.lastTick[event.Symbol]
	e.mu.Unlock()
	if !ok {
		e.logger.Warn("no last tick available", "symbol", event.Symbol)
		return
	}

	// 获取持仓状态（暂时使用假数据，后续对接 Portfolio Manager）
	portfolio := PortfolioState{
		CurrentPosition: 0,
		AvgCost:         0,
		MaxLimit:        10,
		AccountBalance:  50000,
	}
	if e.getPortfolio != nil {
		if p, err := e.getPortfolio(event.Symbol); err == nil {
			portfolio = p
		} else {
			e.logger.Error("failed to get portfolio", "symbol", event.Symbol, "err", err)
			return
		}
	}

	// 构建 AI 上下文
	aiCtx, err := e.ctxBuilder.BuildContext(tick, portfolio)
	if err != nil {
		e.logger.Error("context building failed", "symbol", event.Symbol, "err", err)
		return
	}

	// 序列化 JSON 作为 user content（后续步骤可在此调用 AI）
	userContent, err := aiCtx.ToJSON()
	if err != nil {
		e.logger.Error("context marshalling failed", "symbol", event.Symbol, "err", err)
		return
	}
	e.logger.Info("AI context ready",
		"symbol", event.Symbol,
		"reason", event.Reason,
		"json_size", len(userContent),
	)

	// e.logger.Info("raw AI request", "symbol", event.Symbol, "content", userContent)

	// 系统提示词
	systemPrompt := `You are a disciplined quantitative trader. Analyze the provided structured market data and return ONLY a JSON object with the following fields: action (BUY/SELL/HOLD), quantity (number), order_type (MARKET/LIMIT), limit_price (if LIMIT), reason (string), confidence (0.0-1.0), stop_loss_suggestion, take_profit_suggestion. Do not add any other text.`

	// 调用 DeepSeek API（可重试）
	var aiContent string
	callStart := time.Now() // 开始计时

	for retry := 0; retry < 2; retry++ {
		aiContent, err = e.aiClient.ChatCompletion(systemPrompt, userContent)
		if err == nil {
			break
		}
		e.logger.Warn("AI call failed, retrying", "symbol", event.Symbol, "err", err, "retry", retry+1)
		time.Sleep(retryDelay)
	}

	callDuration := time.Since(callStart) // 结束计时

	if err != nil {
		e.logger.Error("AI call ultimately failed",
			"symbol", event.Symbol,
			"err", err,
			"duration", callDuration.String(), // 记录失败调用耗时
		)
		return
	}

	// 打印 AI 调用成功后的耗时
	e.logger.Info("AI call succeeded",
		"symbol", event.Symbol,
		"duration", callDuration.String(),
		"content", aiContent,
	)

	// 后处理：解析并安全检查
	price := (tick.Bid + tick.Ask) / 2
	decision, modified, err := e.postProcessor.Process(aiContent, portfolio.CurrentPosition, portfolio.MaxLimit, portfolio.AccountBalance, price)
	if err != nil {
		e.logger.Error("post-processing failed", "symbol", event.Symbol, "err", err)
		return
	}

	e.logAIDecision(ctx, event, userContent, aiContent, decision, modified, int(callDuration.Milliseconds()), tick, err)

	if modified {
		e.logger.Warn("AI decision was modified", "symbol", event.Symbol, "action", decision.Action, "reason", decision.Reason)
	}
	if decision.Action == actionHold {
		e.logger.Info("final decision: HOLD", "symbol", event.Symbol)
		return
	}

	e.logger.Info("final trading decision",
		"symbol", event.Symbol,
		"action", decision.Action,
		"quantity", decision.Quantity,
		"confidence", decision.Confidence,
		"reason", decision.Reason,
	)

	// 下单
	orderReq := broker.OrderRequest{
		Symbol:    event.Symbol,
		Action:    decision.Action,
		Quantity:  decision.Quantity,
		OrderType: decision.OrderType,
		Price:     decision.LimitPrice,
		Reason:    decision.Reason,
	}
	resp, err := e.broker.PlaceOrder(ctx, orderReq)
	if err != nil {
		e.logger.Error("broker order failed", "symbol", event.Symbol, "err", err)
	} else {
		e.logger.Info("order placed",
			"orderID", resp.OrderID,
			"status", resp.Status,
			"filledQty", resp.FilledQty,
			"commission", resp.Commission,
		)
	}
}

func (e *Engine) logAIDecision(ctx context.Context, event SignalEvent, requestJSON, aiContent string, decision AIDecision, modified bool, callDuration int, tick TickData, aiErr error) {
	if e.aiLogRepo == nil {
		return
	}

	if aiErr != nil {
		aiContent = fmt.Sprintf("AI error: %v", aiErr)
	}

	if err := e.aiLogRepo.Insert(ctx, event.Symbol, tick.Time, event.Reason, requestJSON, aiContent, decision, modified, callDuration, tick); err != nil {
		e.logger.Error("failed to log AI decision", "symbol", event.Symbol, "err", err)
	} else {
		e.logger.Debug("AI decision logged to database", "symbol", event.Symbol)
	}
}
