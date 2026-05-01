// filter.go
package decision

import (
	"log"
	"sync"
	"time"

	"github.com/wancm/trader-bot/internal/marketdata"
)

type SignalFilter struct {
	rules       []Rule
	lastSignal  map[string]time.Time // symbol -> 上次触发时间
	mu          sync.Mutex
	signalChan  chan<- SignalEvent   // 输出通道，由 Decision Engine 上层传入
}

// NewSignalFilter 创建一个信号过滤器
func NewSignalFilter(rules []Rule, signalChan chan<- SignalEvent) *SignalFilter {
	return &SignalFilter{
		rules:      rules,
		lastSignal: make(map[string]time.Time),
		signalChan: signalChan,
	}
}

// ProcessTick 处理每一个 tick，检查所有规则
func (f *SignalFilter) ProcessTick(tick marketdata.Tick) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
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
		log.Printf("[SignalFilter] Triggered: %s %s", tick.Symbol, reason)
		// 非阻塞发送，避免拖慢行情处理
		select {
		case f.signalChan <- event:
		default:
			log.Printf("[SignalFilter] signal channel full, dropped event for %s", tick.Symbol)
		}
	}
}