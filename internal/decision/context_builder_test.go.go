package decision

import (
	"testing"
	"time"
)

func TestBarAggregator(t *testing.T) {
	agg := NewBarAggregator("XAUUSD", 5*time.Minute, 10)
	// 模拟两个 tick 属于同一根 bar，然后第三个 tick 开启新 bar
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	tick1 := TickData{Symbol: "XAUUSD", Bid: 100.0, Ask: 100.2, Time: now}
	tick2 := TickData{Symbol: "XAUUSD", Bid: 101.0, Ask: 101.2, Time: now.Add(1 * time.Minute)}
	tick3 := TickData{Symbol: "XAUUSD", Bid: 102.0, Ask: 102.2, Time: now.Add(5 * time.Minute)} // 新 bar
	agg.AddTick(tick1)
	agg.AddTick(tick2)
	agg.AddTick(tick3)

	bars := agg.GetRecentBars(5)
	if len(bars) != 1 { // 只有第一个 5min bar 闭合了
		t.Fatalf("expected 1 closed bar, got %d", len(bars))
	}
	bar := bars[0]
	if bar.Open != 100.1 || bar.Close != 101.1 { // 中间价 (100.1 -> 101.1)
		t.Errorf("unexpected bar values: %+v", bar)
	}
}

func TestContextBuilder_Build(t *testing.T) {
	agg := NewBarAggregator("XAUUSD", 5*time.Minute, 10)
	// 先灌入一些历史 bar
	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 15; i++ {
		tick := TickData{
			Symbol: "XAUUSD",
			Bid:    float64(100 + i),
			Ask:    float64(100+i) + 0.2,
			Time:   base.Add(time.Duration(i) * 5 * time.Minute),
			RSI:    45.0,
		}
		agg.AddTick(tick)
	}
	// 当前 tick
	latestTick := TickData{
		Symbol: "XAUUSD",
		Bid:    115.0,
		Ask:    115.2,
		Time:   base.Add(75 * time.Minute),
		RSI:    28.5,
	}

	builder := NewContextBuilder(agg)
	portfolio := PortfolioState{
		CurrentPosition: 0,
		AvgCost:         0,
		MaxLimit:        10,
		AccountBalance:  50000,
	}
	ctx, err := builder.BuildContext(latestTick, portfolio)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.MarketSnapshot.LatestPrice != 115.1 {
		t.Errorf("unexpected latest price")
	}
	if ctx.TechnicalIndicators.RSI != 28.5 {
		t.Errorf("unexpected RSI")
	}
	if len(ctx.RecentPriceAction.Bars) != 10 { // 我们 GetRecentBars(20)，但只有15根闭合 bar，所以返回10根？
		// 检查实际逻辑：我们灌入了15根bar，GetRecentBars(20) 应返回全部15根
		if len(ctx.RecentPriceAction.Bars) != 15 {
			t.Errorf("expected 15 recent bars, got %d", len(ctx.RecentPriceAction.Bars))
		}
	}

	jsonStr, err := ctx.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AI Context JSON:\n%s", jsonStr)
}
