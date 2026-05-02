package app_shared

import (
	"flag"
	"log/slog"
	"os"
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
