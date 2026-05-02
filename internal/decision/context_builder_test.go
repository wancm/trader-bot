// // decision/context_builder_test.go
package decision

import (
	"testing"
	"time"
)

// mockMT5Provider 模拟历史数据
type mockMT5Provider struct {
	bars map[string][]MT5Bar
}

func (m *mockMT5Provider) FetchHistoricalBars(symbol, timeframe string, count int) ([]MT5Bar, error) {
	key := symbol + "_" + timeframe
	bars := m.bars[key]
	if len(bars) > count {
		bars = bars[:count]
	}
	return bars, nil
}

func TestContextBuilder_Build(t *testing.T) {
	// 准备模拟数据
	daily := []MT5Bar{
		{Timestamp: time.Now().Add(-48 * time.Hour).Unix(), Open: 100, High: 105, Low: 99, Close: 104},
		{Timestamp: time.Now().Add(-24 * time.Hour).Unix(), Open: 104, High: 108, Low: 103, Close: 107},
	}
	hourly := []MT5Bar{
		{Timestamp: time.Now().Add(-2 * time.Hour).Unix(), Open: 106, High: 107, Low: 105.5, Close: 106.5},
		{Timestamp: time.Now().Add(-1 * time.Hour).Unix(), Open: 106.5, High: 108, Low: 106, Close: 107.5},
	}

	provider := &mockMT5Provider{
		bars: map[string][]MT5Bar{
			"XAUUSD_1d": daily,
			"XAUUSD_1h": hourly,
		},
	}

	builder := NewContextBuilder(provider)
	tick := TickData{Symbol: "XAUUSD", Bid: 107.4, Ask: 107.6, RSI: 45}
	portfolio := PortfolioState{
		CurrentPosition: 0,
		MaxLimit:        10,
		AccountBalance:  50000,
	}

	ctx, err := builder.BuildContext(tick, portfolio)
	if err != nil {
		t.Fatal(err)
	}

	if ctx.MarketSnapshot.LatestPrice != 107.5 {
		t.Errorf("unexpected latest price: %f", ctx.MarketSnapshot.LatestPrice)
	}
	if len(ctx.MultiTimeframeData.Daily) != 2 {
		t.Errorf("expected 2 daily bars, got %d", len(ctx.MultiTimeframeData.Daily))
	}
	if ctx.RecentPriceAction.Timeframe != "1h" {
		t.Errorf("expected timeframe 1h, got %s", ctx.RecentPriceAction.Timeframe)
	}

	jsonStr, err := ctx.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AI Context JSON:\n%s", jsonStr)
}
