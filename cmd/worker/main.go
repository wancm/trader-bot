package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/wancm/trader-bot/app_shared"
	"github.com/wancm/trader-bot/internal/broker"
	"github.com/wancm/trader-bot/internal/decision"
	"github.com/wancm/trader-bot/internal/marketdata"
	"github.com/wancm/trader-bot/internal/marketdata/mt5"
	"github.com/wancm/trader-bot/repository"
)

func main() {

	wd, _ := os.Getwd()
	fmt.Println("current dir:", wd)

	// Load the first .env we can find. godotenv.Load(p1, p2, ...) returns the
	// first error it hits, which trips even when an earlier path loaded
	// successfully — so try each path independently and stop on the first hit.
	envCandidates := []string{
		"configs/.env",
		"../configs/.env",
		"../../configs/.env",
		"../../../configs/.env",
	}
	envLoaded := false
	for _, p := range envCandidates {
		if err := godotenv.Load(p); err == nil {
			app_shared.AppLogger.Info("loaded env file", "path", p)
			envLoaded = true
			break
		}
	}
	if !envLoaded {
		app_shared.AppLogger.Info(".env not found in any candidate path, using system environment variables")
	}

	ctx := context.Background()

	repository.Init(ctx) // 初始化数据库连接池

	paperBroker := broker.NewPaperBroker(repository.PG_Pool) // 创建模拟券商实例

	addr := flag.String("mt5-addr", envOr("MT5_ADDR", "127.0.0.1:5000"), "TCP address for the mt5-bridge tick stream (env: MT5_ADDR)")
	mt5BaseURL := flag.String("mt5-base-url", envOr("MT5_BASE_URL", "http://localhost:18812"), "HTTP base URL of the MT5 gateway (env: MT5_BASE_URL)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener := &mt5.Listener{Logger: app_shared.AppLogger}

	// 创建信号通道
	signalChan := make(chan decision.SignalEvent, 100)

	// 规则配置
	rules := []decision.Rule{
		decision.RSIRule{Threshold: 30, Above: false, CooldownDur: 5 * time.Minute},
		decision.RSIRule{Threshold: 70, Above: true, CooldownDur: 5 * time.Minute},
	}

	// 从环境变量获取 AI API Key
	aiAPIKey := os.Getenv("DEEPSEEK_API_KEY")
	if aiAPIKey == "" {
		app_shared.AppLogger.Error("DEEPSEEK_API_KEY not set")
		os.Exit(1)
	}

	// 实例化 Facade
	engine := decision.NewEngine(
		rules,
		signalChan,
		*mt5BaseURL,
		aiAPIKey,
		"https://api.deepseek.com",
		"deepseek-v4-flash",
		false,
		0.5,
		app_shared.AppLogger,
		repository.PG_Pool, // 传入数据库连接池
		paperBroker,        // 传入 broker 实例
	)

	// 启动信号消费（包含上下文构建）
	engine.Start(ctx)

	listener.OnTick = func(tick marketdata.Tick) {
		// 1. 更新 broker 的最新报价
		paperBroker.UpdatePrices(tick.Symbol, tick.Bid, tick.Ask)

		// 2. 转成 decision.TickData 并推入引擎
		dt := decision.TickData{
			Symbol: tick.Symbol,
			Bid:    tick.Bid,
			Ask:    tick.Ask,
			Time:   app_shared.UnixToTime(tick.Timestamp),
			RSI:    tick.RSI,
			// 如果有更多字段，按需映射
		}
		engine.ProcessTick(ctx, dt)
	}

	if err := listener.ListenAndServe(ctx, *addr); err != nil {
		app_shared.AppLogger.Error("mt5 listener exited with error", "err", err)
		os.Exit(1)
	}

	// 等待终止信号
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	// cancel()
	stop()

	app_shared.AppLogger.Info("trader-bot shutdown complete")
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
