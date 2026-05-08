package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/wancm/trader-bot/internal/broker"
	"github.com/wancm/trader-bot/internal/decision"
	"github.com/wancm/trader-bot/internal/marketdata"
	"github.com/wancm/trader-bot/internal/marketdata/mt5"
	"github.com/wancm/trader-bot/internal/shared"
)

func main() {
	// Load the first .env we can find. godotenv.Load(p1, p2, ...) returns the
	// first error it hits, which trips even when an earlier path loaded
	// successfully — so try each path independently and stop on the first hit.
	// Must load before registering flags so envOr reads the correct defaults.
	var envPath string
	for _, p := range []string{".env", "../.env", "../../.env", "../../../.env"} {
		if err := godotenv.Load(p); err == nil {
			envPath = p
			break
		}
	}

	addr := flag.String("mt5-addr", envOr("MT5_ADDR", "127.0.0.1:5000"), "TCP address for the mt5-bridge tick stream (env: MT5_ADDR)")
	mt5BaseURL := flag.String("mt5-base-url", envOr("MT5_BASE_URL", "http://localhost:18812"), "HTTP base URL of the MT5 gateway (env: MT5_BASE_URL)")
	logFormat := flag.String("log-format", envOr("LOG_FORMAT", "text"), "log format: text or json (env: LOG_FORMAT)")
	loggerWSURL    := flag.String("logger-ws-url", envOr("LOGGER_WS_URL", "ws://127.0.0.1:9500"), "logger service WebSocket URL (env: LOGGER_WS_URL)")
	brokerTickURL  := flag.String("broker-tick-url", envOr("BROKER_TICK_URL", "ws://127.0.0.1:8086/ticks"), "broker-manager tick WebSocket URL (env: BROKER_TICK_URL)")
	flag.Parse()

	wsLogger, logFwd := shared.NewLoggerWithWS(*logFormat, *loggerWSURL, "trader-bot")
	shared.AppLogger = wsLogger
	logger := shared.AppLogger
	if envPath != "" {
		logger.Info("loaded env file", "path", envPath)
	} else {
		logger.Info(".env not found in any candidate path, using system environment variables")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go logFwd.Run(ctx)

	// 推送行情到 broker-manager 的 tick 端点
	brokerFwd := shared.NewTickForwarder(*brokerTickURL)
	go brokerFwd.Run(ctx)

	// 初始化数据库连接池
	dbConn := os.Getenv("DB_CONN")
	if dbConn == "" {
		logger.Error("DB_CONN environment variable is required")
		os.Exit(1)
	}
	pool, err := pgxpool.New(ctx, dbConn)
	if err != nil {
		logger.Error("database connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// 创建模拟券商实例
	paperBroker := broker.NewPaperBroker(pool)

	// 从环境变量获取 AI API Key
	aiAPIKey := os.Getenv("DEEPSEEK_API_KEY")
	if aiAPIKey == "" {
		logger.Error("DEEPSEEK_API_KEY not set")
		os.Exit(1)
	}

	// 创建信号通道
	signalChan := make(chan decision.SignalEvent, 100)

	// 规则配置
	rules := []decision.Rule{
		decision.RSIRule{Threshold: 30, Above: false, CooldownDur: 5 * time.Minute},
		decision.RSIRule{Threshold: 70, Above: true, CooldownDur: 5 * time.Minute},
	}

	// 实例化 Facade
	engine := decision.NewEngine(
		rules,
		signalChan,
		*mt5BaseURL,
		aiAPIKey,
		"https://api.deepseek.com",
		"deepseek-chat",
		false,
		0.5,
		logger,
		pool,
		paperBroker,
	)

	engine.SetAILogForwarder(func(msg string, payload any, t time.Time) {
		logFwd.SendEntry("ai_decisions", msg, t, payload)
	})

	engine.SetOrderForwarder(func(msg string, payload any, t time.Time) {
		logFwd.SendEntry("order", msg, t, payload)
	})

	// 启动信号消费（包含上下文构建）
	engine.Start(ctx)

	// tickLogThrottle limits tick entries sent to the logger to at most once per
	// second per symbol so the queue is not overwhelmed during backtesting.
	var (
		tickLogMu   sync.Mutex
		tickLogLast = make(map[string]time.Time)
	)
	shouldLogTick := func(symbol string) bool {
		tickLogMu.Lock()
		defer tickLogMu.Unlock()
		now := time.Now()
		if now.Sub(tickLogLast[symbol]) >= time.Second {
			tickLogLast[symbol] = now
			return true
		}
		return false
	}

	listener := &mt5.Listener{Logger: logger}
	listener.OnTick = func(tick marketdata.Tick) {
		tickTime := shared.UnixToTime(tick.Timestamp)

		// 1. 更新 broker 的最新报价
		paperBroker.UpdatePrices(tick.Symbol, tick.Bid, tick.Ask)

		// 2. 转成 decision.TickData 并推入引擎
		dt := decision.TickData{
			Symbol: tick.Symbol,
			Bid:    tick.Bid,
			Ask:    tick.Ask,
			Time:   tickTime,
			RSI:    tick.RSI,
		}
		engine.ProcessTick(ctx, dt)

		// 3. 推送行情到 logger service — throttled to 1/s per symbol
		if shouldLogTick(tick.Symbol) {
			logFwd.SendEntry("tick",
				fmt.Sprintf("%s bid=%.5f ask=%.5f rsi=%.2f", tick.Symbol, tick.Bid, tick.Ask, tick.RSI),
				tickTime,
				tick,
			)
		}

		// 4. 推送行情到 broker-manager
		brokerFwd.Send(tick)
	}

	for {
		if err := listener.ListenAndServe(ctx, *addr); err != nil {
			logger.Error("mt5 listener failed, retrying in 5s", "err", err)
		}
		if ctx.Err() != nil {
			return
		}
		time.Sleep(5 * time.Second)
	}
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
