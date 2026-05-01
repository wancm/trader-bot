package decision

import (
	"time"

	"github.com/wancm/trader-bot/internal/marketdata"
)

// Rule 接口代表一条信号规则
type Rule interface {
	// Evaluate 检查 TickData 是否满足规则条件，返回 true/false 及原因字符串
	Evaluate(tick marketdata.Tick) (bool, string)
	// Cooldown 返回本规则的最小冷却时间
	Cooldown() time.Duration
}