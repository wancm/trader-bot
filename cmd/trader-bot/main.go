package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/wancm/trader-bot/internal/decision"
	"github.com/wancm/trader-bot/internal/marketdata"
	"github.com/wancm/trader-bot/internal/marketdata/mt5"
)

func main() {
	// Load .env if present. Missing file is not an error — env vars and CLI
	// flags still work when the file is absent.
	_ = godotenv.Load()

	addr := flag.String("mt5-addr", envOr("MT5_ADDR", "127.0.0.1:5000"), "TCP address for the mt5-bridge tick stream (env: MT5_ADDR)")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener := &mt5.Listener{Logger: logger}

	// 创建信号通道
	signalChan := make(chan decision.SignalEvent, 100)

	// 规则配置
	rules := []decision.Rule{
		decision.RSIRule{Threshold: 30, Above: false, CooldownDuration: 5 * time.Minute},
		decision.RSIRule{Threshold: 70, Above: true, CooldownDuration: 5 * time.Minute},
	}

	// 实例化 Facade
	engine := decision.NewEngine(rules, signalChan)

	listener.OnTick = func(tick marketdata.Tick) {
		// 把 marketdata.Tick 转成 decision.TickData
		dt := decision.TickData{
			Symbol: tick.Symbol,
			Bid:    tick.Bid,
			Ask:    tick.Ask,
			Time:   time.Unix(tick.Timestamp, 0),
			RSI:    tick.RSI,
			// 如果有更多字段，按需映射
		}
		engine.ProcessTick(dt)
	}

	if err := listener.ListenAndServe(ctx, *addr); err != nil {
		logger.Error("mt5 listener exited with error", "err", err)
		os.Exit(1)
	}

	logger.Info("trader-bot shutdown complete")
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
