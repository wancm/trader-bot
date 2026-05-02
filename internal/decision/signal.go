// decision/signal.go
package decision

import "time"

// TickData 代表从 Market Data Hub 推送的一帧行情快照
type TickData struct {
	Symbol string
	Bid    float64
	Ask    float64
	Time   time.Time
	RSI    float64 // 已计算好的 RSI(14)，后续可扩展更多指标
}

// SignalEvent 是信号过滤器输出的触发事件
type SignalEvent struct {
	Symbol    string
	Reason    string // 触发原因，例如 "RSI oversold (28.5 < 30)"
	Timestamp time.Time
}

// Rule 接口代表一条交易信号规则
type Rule interface {
	// Evaluate 检查 TickData 是否满足规则条件，返回是否触发以及原因字符串
	Evaluate(tick TickData) (bool, string)
	// Cooldown 返回本规则的最小冷却时间
	Cooldown() time.Duration
}
