package decision

import "time"

// SignalEvent 是信号过滤器输出的触发事件
type SignalEvent struct {
	Symbol    string
	Reason    string   // 触发原因，例如 "RSI oversold (28.5 < 30)"
	Timestamp time.Time
}