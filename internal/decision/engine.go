// decision/engine.go
package decision

import (
	"context"
	"log/slog"
	"sync"
)

// Engine 是决策引擎的外观，协调信号过滤、上下文构建等
type Engine struct {
	signalFilter *SignalFilter
	mt5Client    *MT5Client
	ctxBuilder   *ContextBuilder
	getPortfolio func(symbol string) (PortfolioState, error)
	lastTick     map[string]TickData
	mu           sync.Mutex
	signalChan   chan SignalEvent
	logger       *slog.Logger
}

// NewEngine 创建一个新的决策引擎
func NewEngine(rules []Rule, signalChan chan SignalEvent, mt5BaseURL string, logger *slog.Logger) *Engine {
	client := NewMT5Client(mt5BaseURL)
	return &Engine{
		signalFilter: NewSignalFilter(rules, signalChan, logger),
		mt5Client:    client,
		ctxBuilder:   NewContextBuilder(client),
		lastTick:     make(map[string]TickData),
		signalChan:   signalChan,
		logger:       logger,
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
	// TODO: 调用 AI 客户端，处理回应，发送订单
}
