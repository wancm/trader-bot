// decision/context_builder.go
package decision

import (
	"encoding/json"
	"fmt"
)

// AIContext 是发送给 DeepSeek 的完整 JSON 结构
type AIContext struct {
	TraderProfile       TraderProfile       `json:"tp"`
	MarketSnapshot      MarketSnapshot      `json:"ms"`
	TechnicalIndicators TechnicalIndicators `json:"ti"`
	RecentPriceAction   RecentPriceAction   `json:"rpa"`
	MultiTimeframeData  MultiTimeframeData  `json:"mtf"`
	PortfolioState      PortfolioState      `json:"ps"`
	AnalysisGuidelines  AnalysisGuidelines  `json:"ag"`
}

type TraderProfile struct {
	AccountType   string `json:"at"` // cash / margin
	AllowShort    bool   `json:"as"` // allow short selling
	RiskTolerance string `json:"rt"` // low / medium / high
}

type MarketSnapshot struct {
	Symbol      string  `json:"sym"` // symbol
	LatestPrice float64 `json:"lp"`  // latest price
	Bid         float64 `json:"b"`
	Ask         float64 `json:"a"`
}

type TechnicalIndicators struct {
	RSI float64 `json:"rsi"` // rsi_14 直接叫 rsi，AI 能懂
}

type RecentBar struct {
	Open  float64 `json:"o"`
	High  float64 `json:"h"`
	Low   float64 `json:"l"`
	Close float64 `json:"c"`
}

type RecentPriceAction struct {
	Timeframe string      `json:"tf"` // e.g. "5m", "1h"
	Bars      []RecentBar `json:"b"`  // bars -> b
}

type MultiTimeframeData struct {
	Daily  []RecentBar `json:"d"` // daily
	Hourly []RecentBar `json:"h"` // hourly
}

type PortfolioState struct {
	CurrentPosition int     `json:"cp"` // current position
	AvgCost         float64 `json:"ac"` // average cost
	MaxLimit        int     `json:"ml"` // max position limit
	AccountBalance  float64 `json:"ab"` // account balance
}

type AnalysisGuidelines struct {
	DataDependency    string `json:"dd"`    // base on provided data only
	NoFabrication     string `json:"nf"`    // do not fabricate values
	UncertaintyExpr   string `json:"ue"`    // if uncertain, say HOLD
	ReasoningFormat   string `json:"rf"`    // step-by-step reasoning
	AvoidGeneralities string `json:"avoid"` // avoid vague language
}

// ContextBuilder 使用 MT5 客户端构建 AI 上下文
type ContextBuilder struct {
	// mt5Client *MT5Client
	dataProvider HistoricalDataProvider
}

// NewContextBuilder 创建上下文构建器
func NewContextBuilder(provider HistoricalDataProvider) *ContextBuilder {
	return &ContextBuilder{dataProvider: provider}
}

// BuildContext 根据最新 Tick 和持仓状态构建 AI 上下文
func (cb *ContextBuilder) BuildContext(tick TickData, portfolio PortfolioState) (AIContext, error) {

	// TODO: 这里可以并行拉取不同周期的历史数据以加快速度

	// 拉取多周期历史 K 线
	dailyBars, err := cb.dataProvider.FetchHistoricalBars(tick.Symbol, "1d", 70)
	if err != nil {
		return AIContext{}, fmt.Errorf("failed to fetch daily bars: %w", err)
	}

	hourlyBars, err := cb.dataProvider.FetchHistoricalBars(tick.Symbol, "1h", 150)
	if err != nil {
		return AIContext{}, fmt.Errorf("failed to fetch hourly bars: %w", err)
	}

	ctx := AIContext{
		TraderProfile: TraderProfile{
			AccountType:   "cash",
			AllowShort:    false,
			RiskTolerance: "medium",
		},
		MarketSnapshot: MarketSnapshot{
			Symbol:      tick.Symbol,
			LatestPrice: (tick.Bid + tick.Ask) / 2,
			Bid:         tick.Bid,
			Ask:         tick.Ask,
		},
		TechnicalIndicators: TechnicalIndicators{
			RSI: tick.RSI,
		},
		RecentPriceAction: RecentPriceAction{
			Timeframe: "1h",
			Bars:      convertMT5BarsToRecent(hourlyBars),
		},
		MultiTimeframeData: MultiTimeframeData{
			Daily:  convertMT5BarsToRecent(dailyBars),
			Hourly: convertMT5BarsToRecent(hourlyBars),
		},
		PortfolioState:     portfolio,
		AnalysisGuidelines: defaultAnalysisGuidelines(),
	}
	return ctx, nil
}

// ToJSON 将上下文序列化为 JSON 字符串
func (ctx AIContext) ToJSON() (string, error) {
	b, err := json.Marshal(ctx)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// convertMT5BarsToRecent 将 MT5Bar 转为通用 RecentBar
func convertMT5BarsToRecent(bars []MT5Bar) []RecentBar {
	out := make([]RecentBar, len(bars))
	for i, b := range bars {
		out[i] = RecentBar{
			Open:  b.Open,
			High:  b.High,
			Low:   b.Low,
			Close: b.Close,
		}
	}
	return out
}

// defaultAnalysisGuidelines 返回抑制幻觉的指引
func defaultAnalysisGuidelines() AnalysisGuidelines {
	return AnalysisGuidelines{
		DataDependency:    "Base all observations strictly on provided data.",
		NoFabrication:     "Do not invent any values or patterns not present.",
		UncertaintyExpr:   "If uncertain, set confidence < 0.5 and recommend HOLD.",
		ReasoningFormat:   "Step-by-step referencing specific numeric values.",
		AvoidGeneralities: "Avoid vague statements, always cite exact price/indicator values.",
	}
}
