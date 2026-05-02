// decision/rsi_rule.go
package decision

import (
	"fmt"
	"time"
)

// RSIRule 根据 RSI 阈值触发信号
type RSIRule struct {
	Threshold   float64       // 阈值，例如 30 或 70
	Above       bool          // true: RSI > Threshold 触发（超买），false: RSI < Threshold 触发（超卖）
	CooldownDur time.Duration // 冷却时间
}

// Evaluate 判断是否满足规则
func (r RSIRule) Evaluate(tick TickData) (bool, string) {
	if r.Above {
		if tick.RSI > r.Threshold {
			reason := fmt.Sprintf("RSI overbought (%.2f > %.0f)", tick.RSI, r.Threshold)
			return true, reason
		}
	} else {
		if tick.RSI < r.Threshold {
			reason := fmt.Sprintf("RSI oversold (%.2f < %.0f)", tick.RSI, r.Threshold)
			return true, reason
		}
	}
	return false, ""
}

// Cooldown 返回规则的冷却时间
func (r RSIRule) Cooldown() time.Duration {
	return r.CooldownDur
}
