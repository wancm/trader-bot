package decision

import (
	"fmt"
	"time"

	"github.com/wancm/trader-bot/internal/marketdata"
)

type RSIRule struct {
	Threshold        float64 // 阈值，例如 30 表示超卖，70 表示超买
	Above            bool    // true: RSI > Threshold 触发（超买），false: RSI < Threshold 触发（超卖）
	CooldownDuration time.Duration
}

func (r RSIRule) Evaluate(tick marketdata.Tick) (bool, string) {
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

func (r RSIRule) Cooldown() time.Duration {
	return r.CooldownDuration
}
