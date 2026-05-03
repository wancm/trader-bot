package decision

import (
	"encoding/json"
	"fmt"
	"math"
)

// PostProcessor 负责解析 AI 返回的 JSON 并进行安全修正
type PostProcessor struct {
	AllowShort        bool
	ConfidenceMinimum float64
}

// NewPostProcessor 创建后处理器
func NewPostProcessor(allowShort bool, minConfidence float64) *PostProcessor {
	return &PostProcessor{
		AllowShort:        allowShort,
		ConfidenceMinimum: minConfidence,
	}
}

// Process 解析 AI 内容，返回决策及是否被修正
func (p *PostProcessor) Process(aiContent string, currentShares int, maxLimit int, balance float64, price float64) (AIDecision, bool, error) {
	var raw AIDecision
	if err := json.Unmarshal([]byte(aiContent), &raw); err != nil {
		return AIDecision{}, false, fmt.Errorf("parse ai decision: %w", err)
	}

	modified := false

	// 1. 置信度低于阈值 → HOLD
	if raw.Confidence < p.ConfidenceMinimum {
		raw.Action = "HOLD"
		raw.Quantity = 0
		raw.Reason = "Confidence below threshold, forcing HOLD"
		modified = true
		return raw, modified, nil
	}

	// 2. 禁止做空：SELL 但无持仓 → HOLD
	if raw.Action == "SELL" && currentShares <= 0 && !p.AllowShort {
		raw.Action = "HOLD"
		raw.Quantity = 0
		raw.Reason = "Short selling not allowed, forcing HOLD"
		modified = true
		return raw, modified, nil
	}

	// 3. 卖出数量不能超过持仓
	if raw.Action == "SELL" && raw.Quantity > float64(currentShares) {
		raw.Quantity = float64(currentShares)
		raw.Reason = fmt.Sprintf("Sell quantity adjusted to max %d shares", currentShares)
		modified = true
		if raw.Quantity == 0 {
			raw.Action = "HOLD"
		}
	}

	// 4. 买入量检查：不能超过余额或上限
	if raw.Action == "BUY" {
		maxBuyShares := maxLimit - currentShares
		if maxBuyShares <= 0 {
			raw.Action = "HOLD"
			raw.Quantity = 0
			raw.Reason = "Already at max position limit, forcing HOLD"
			modified = true
		} else {
			affordable := int(math.Min(float64(maxBuyShares), math.Floor(balance/price)))
			if int(raw.Quantity) > affordable {
				raw.Quantity = float64(affordable)
				raw.Reason = fmt.Sprintf("Buy quantity adjusted to affordable: %d", affordable)
				modified = true
			}
			if raw.Quantity <= 0 {
				raw.Action = "HOLD"
				raw.Reason = "Insufficient funds for minimum buy"
				modified = true
			}
		}
	}

	// 5. 若 Action 无效，则改为 HOLD
	if raw.Action != "BUY" && raw.Action != "SELL" && raw.Action != "HOLD" {
		raw.Action = "HOLD"
		raw.Quantity = 0
		raw.Reason = "Invalid action, forcing HOLD"
		modified = true
	}

	return raw, modified, nil
}
