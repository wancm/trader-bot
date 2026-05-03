package app_shared

import (
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"time"
)

var AppLogger = newLogger()

func newLogger() *slog.Logger {
	logFmt := flag.String("log-format", envOr("LOG_FORMAT", "text"), "log format: text or json (env: LOG_FORMAT)")
	flag.Parse()

	var handler slog.Handler
	switch *logFmt {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, nil)
	default:
		handler = slog.NewTextHandler(os.Stdout, nil)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// UnixToTime 将 Unix 秒转换为 time.Time
func UnixToTime(unixSec int64) time.Time {
	return time.Unix(unixSec, 0)
}

// ToJsonIndent 将任意结构体序列化为带缩进的 JSON 字符串，便于日志记录
func ToJsonIndent(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}

	return string(data)
}
