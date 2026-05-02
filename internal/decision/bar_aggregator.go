package decision

import (
	"sync"
	"time"
)

// Bar 代表一根完整的 OHLCV K 线
type Bar struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// BarAggregator 根据 Tick 聚合指定周期的 K 线，并维护最近 N 根闭合 Bar
type BarAggregator struct {
	mu         sync.Mutex
	period     time.Duration
	maxBars    int
	current    *Bar  // 当前未闭合的 Bar
	closedBars []Bar // 已闭合的 Bar，最新的在后面
	symbol     string
}

// NewBarAggregator 创建一个新的 Bar 聚合器
func NewBarAggregator(symbol string, period time.Duration, maxBars int) *BarAggregator {
	return &BarAggregator{
		symbol:     symbol,
		period:     period,
		maxBars:    maxBars,
		closedBars: make([]Bar, 0, maxBars),
	}
}

// AddTick 喂入一个 Tick，更新当前 Bar 并可能闭合
func (b *BarAggregator) AddTick(tick TickData) {
	b.mu.Lock()
	defer b.mu.Unlock()

	price := (tick.Bid + tick.Ask) / 2.0 // 取中间价，也可按需用 Bid 或 Ask
	truncTime := tick.Time.Truncate(b.period)

	if b.current == nil || !b.current.Time.Equal(truncTime) {
		// 闭合前一根 Bar
		if b.current != nil {
			b.addClosedBar(*b.current)
		}
		// 开启新 Bar
		b.current = &Bar{
			Time:  truncTime,
			Open:  price,
			High:  price,
			Low:   price,
			Close: price,
		}
	} else {
		// 更新当前 Bar 的高低收
		if price > b.current.High {
			b.current.High = price
		}
		if price < b.current.Low {
			b.current.Low = price
		}
		b.current.Close = price
	}
	// 这里不处理 Volume，若 Tick 有 Volume 可自行累加
}

func (b *BarAggregator) addClosedBar(bar Bar) {
	b.closedBars = append(b.closedBars, bar)
	// 维持最大数量（保留最新的）
	if len(b.closedBars) > b.maxBars {
		b.closedBars = b.closedBars[len(b.closedBars)-b.maxBars:]
	}
}

// GetRecentBars 返回最近 N 根已闭合的 Bar（从旧到新），可能少于 N 根
func (b *BarAggregator) GetRecentBars(n int) []Bar {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n <= 0 || len(b.closedBars) == 0 {
		return []Bar{}
	}
	start := len(b.closedBars) - n
	if start < 0 {
		start = 0
	}
	out := make([]Bar, len(b.closedBars[start:]))
	copy(out, b.closedBars[start:])
	return out
}
