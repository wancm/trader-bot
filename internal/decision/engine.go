// decision/engine.go
package decision

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine 是决策引擎的外观，协调信号过滤、上下文构建等
type Engine struct {
	signalFilter  *SignalFilter
	mt5Client     *MT5Client
	ctxBuilder    *ContextBuilder
	aiClient      *AIClient      // 新增
	postProcessor *PostProcessor // 新增
	getPortfolio  func(symbol string) (PortfolioState, error)
	lastTick      map[string]TickData
	mu            sync.Mutex
	signalChan    chan SignalEvent
	logger        *slog.Logger
	aiLogRepo     *AILogRepository // 新增
}

// NewEngine 创建一个新的决策引擎
func NewEngine(
	rules []Rule,
	signalChan chan SignalEvent,
	mt5BaseURL, aiAPIKey, aiBaseURL, aiModel string,
	allowShort bool, minConfidence float64,
	logger *slog.Logger,
	dbPool *pgxpool.Pool, // 传入数据库连接池
) *Engine {
	client := NewMT5Client(mt5BaseURL)
	return &Engine{
		signalFilter:  NewSignalFilter(rules, signalChan, logger),
		mt5Client:     client,
		ctxBuilder:    NewContextBuilder(client),
		aiClient:      NewAIClient(aiAPIKey, aiBaseURL, aiModel),
		postProcessor: NewPostProcessor(allowShort, minConfidence),
		lastTick:      make(map[string]TickData),
		signalChan:    signalChan,
		logger:        logger,
		aiLogRepo:     NewAILogRepository(dbPool), // 初始化日志仓库
	}
}

// ProcessTick 接收行情 tick，更新最后 Tick 并触发信号过滤
func (e *Engine) ProcessTick(tick TickData) {
	e.mu.Lock()
	e.lastTick[tick.Symbol] = tick
	e.mu.Unlock()

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

// handleSignal 处理一次信号事件：构建上下文并在下一步调用 AI
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

	// 序列化 JSON（后续步骤可在此调用 AI）
	jsonStr, _ := aiCtx.ToJSON()
	e.logger.Info("AI context ready",
		"symbol", event.Symbol,
		"reason", event.Reason,
		"json_size", len(jsonStr),
	)

	// 将 AI 上下文转为 JSON 作为 user content
	userContent, err := aiCtx.ToJSON()
	if err != nil {
		e.logger.Error("context marshalling failed", "symbol", event.Symbol, "err", err)
		return
	}

	e.logger.Info("raw AI request", "symbol", event.Symbol, "content", userContent)

	// 系统提示词
	systemPrompt := `You are a disciplined quantitative trader. Analyze the provided structured market data and return ONLY a JSON object with the following fields: action (BUY/SELL/HOLD), quantity (number), order_type (MARKET/LIMIT), limit_price (if LIMIT), reason (string), confidence (0.0-1.0), stop_loss_suggestion, take_profit_suggestion. Do not add any other text.`

	// 调用 DeepSeek API（可重试）
	var aiContent string
	for retry := 0; retry < 2; retry++ {
		aiContent, err = e.aiClient.ChatCompletion(systemPrompt, userContent)
		if err == nil {
			break
		}
		e.logger.Warn("AI call failed, retrying", "symbol", event.Symbol, "err", err, "retry", retry+1)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		e.logger.Error("AI call ultimately failed", "symbol", event.Symbol, "err", err)
		return
	}

	// 打印 AI 原始响应
	e.logger.Info("AI response received",
		"symbol", event.Symbol,
		"content_length", len(aiContent),
	)

	// 在拿到 aiContent 之后，立即打印
	e.logger.Debug("raw AI response", "symbol", event.Symbol, "content", aiContent)
	e.logger.Info("raw AI response", "symbol", event.Symbol, "content", aiContent)

	// 后处理：解析并安全检查
	// 需要用到当前持仓与价格等信息，通过上下文中的 tick 数据获取价格
	price := (tick.Bid + tick.Ask) / 2
	decision, modified, err := e.postProcessor.Process(aiContent, portfolio.CurrentPosition, portfolio.MaxLimit, portfolio.AccountBalance, price)
	if err != nil {
		e.logger.Error("post-processing failed", "symbol", event.Symbol, "err", err)
		return
	}

	e.logAIDecision(ctx, event, userContent, aiContent, decision, modified, err)

	if modified {
		e.logger.Warn("AI decision was modified", "symbol", event.Symbol, "action", decision.Action, "reason", decision.Reason)
	}
	if decision.Action == "HOLD" {
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

	e.logger.Info("attempting to log AI decision to DB", "symbol", event.Symbol)

	// TODO: 最后调用 Portfolio Manager 验证订单，然后发送给 Broker Manager
}

func (e *Engine) logAIDecision(ctx context.Context, event SignalEvent, requestJSON, aiContent string, decision AIDecision, modified bool, aiErr error) {
	if e.aiLogRepo == nil {
		return
	}

	if aiErr != nil {
		aiContent = fmt.Sprintf("AI error: %v", aiErr)
	}

	if err := e.aiLogRepo.Insert(ctx, event.Symbol, event.Reason, requestJSON, aiContent, decision, modified); err != nil {
		e.logger.Error("failed to log AI decision", "symbol", event.Symbol, "err", err)
	} else {
		e.logger.Debug("AI decision logged to database", "symbol", event.Symbol)
	}
}
