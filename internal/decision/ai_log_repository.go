// decision/ai_log_repository.go
package decision

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AILogRepository struct {
	pool *pgxpool.Pool
}

func NewAILogRepository(pool *pgxpool.Pool) *AILogRepository {
	return &AILogRepository{pool: pool}
}

func (r *AILogRepository) Insert(ctx context.Context, symbol string, tickTimestamp time.Time, triggerReason, requestJSON, responseRaw string, decision AIDecision, postProcessed bool, callDuration int, tick TickData) error {
	query := `
		INSERT INTO ai_decision_log
			(symbol, tick_timestamp, trigger_reason, request_json, response_raw, decision_json, post_processed, confidence, final_action, final_quantity, call_durations, tick_raw, created_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	tickJSON, _ := json.Marshal(tick)
	_, err := r.pool.Exec(ctx, query,
		symbol,
		tickTimestamp,
		triggerReason,
		requestJSON,                  // 原始 JSON 字符串
		responseRaw,                  // AI 原始输出
		formatDecisionJSON(decision), // 解析后的决策 JSON
		postProcessed,
		decision.Confidence,
		decision.Action,
		decision.Quantity,
		callDuration,
		string(tickJSON),
		time.Now(),
	)
	return err
}

// 将 AIDecision 序列化为 JSON 字符串
func formatDecisionJSON(d AIDecision) string {
	b, _ := json.Marshal(d)
	return string(b)
}
