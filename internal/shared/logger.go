package shared

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// AppLogger is the process-wide structured logger.
// main reassigns it after loading .env if LOG_FORMAT changes.
var AppLogger = NewLogger(os.Getenv("LOG_FORMAT"))

// NewLogger creates a slog.Logger with the given format ("json" or text).
func NewLogger(format string) *slog.Logger {
	opts := handlerOpts()
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}

// UnixToTime 将 Unix 秒转换为 time.Time
func UnixToTime(unixSec int64) time.Time {
	return time.Unix(unixSec, 0).UTC()
}

// ToJsonIndent 将任意结构体序列化为带缩进的 JSON 字符串，便于日志记录
func ToJsonIndent(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}
