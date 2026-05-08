// decision/filter.go
package decision

import (
	"log/slog"
	"sync"
	"time"
)

// SignalFilter 根据规则过滤 Tick，生成 SignalEvent 并推送到 channel
type SignalFilter struct {
	rules      []Rule
	lastSignal map[string]time.Time // symbol → 上次触发时间
	mu         sync.Mutex
	signalChan chan<- SignalEvent
	logger     *slog.Logger
}

// NewSignalFilter 创建信号过滤器
func NewSignalFilter(rules []Rule, signalChan chan<- SignalEvent, logger *slog.Logger) *SignalFilter {
	if logger == nil {
		logger = slog.Default()
	}
	return &SignalFilter{
		rules:      rules,
		lastSignal: make(map[string]time.Time),
		signalChan: signalChan,
		logger:     logger,
	}
}

// ProcessTick 处理每一个 tick，检查所有规则
func (f *SignalFilter) ProcessTick(tick TickData) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now().UTC()
	for _, rule := range f.rules {
		ok, reason := rule.Evaluate(tick)
		if !ok {
			continue
		}
		// 检查冷却时间
		if last, exists := f.lastSignal[tick.Symbol]; exists {
			if now.Sub(last) < rule.Cooldown() {
				continue
			}
		}
		// 触发信号
		f.lastSignal[tick.Symbol] = now
		event := SignalEvent{
			Symbol:    tick.Symbol,
			Reason:    reason,
			Timestamp: now,
		}
		f.logger.Info("signal triggered",
			"symbol", event.Symbol,
			"reason", event.Reason,
		)
		// 非阻塞发送，避免拖慢行情处理
		select {
		case f.signalChan <- event:
		default:
			f.logger.Warn("signal channel full, dropping event",
				"symbol", tick.Symbol,
			)
		}
	}
}
