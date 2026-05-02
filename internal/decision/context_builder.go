// decision/context_builder.go
package decision

import (
	"encoding/json"
	"fmt"
)

// AIContext 是发送给 DeepSeek 的完整 JSON 结构
type AIContext struct {
	TraderProfile       TraderProfile       `json:"trader_profile"`
	MarketSnapshot      MarketSnapshot      `json:"market_snapshot"`
	TechnicalIndicators TechnicalIndicators `json:"technical_indicators"`
	RecentPriceAction   RecentPriceAction   `json:"recent_price_action"`
	MultiTimeframeData  MultiTimeframeData  `json:"multi_timeframe_data"`
	PortfolioState      PortfolioState      `json:"portfolio_state"`
	AnalysisGuidelines  AnalysisGuidelines  `json:"analysis_guidelines"`
}

// 子结构体定义
type TraderProfile struct {
	AccountType   string `json:"account_type"`
	AllowShort    bool   `json:"allow_short"`
	RiskTolerance string `json:"risk_tolerance"`
}

type MarketSnapshot struct {
	Symbol      string  `json:"symbol"`
	LatestPrice float64 `json:"latest_price"`
	Bid         float64 `json:"bid"`
	Ask         float64 `json:"ask"`
}

type TechnicalIndicators struct {
	RSI float64 `json:"rsi_14"`
	// 后续可增加 MACD, SMA 等
}

type RecentBar struct {
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
}

type RecentPriceAction struct {
	Timeframe string      `json:"timeframe"`
	Bars      []RecentBar `json:"bars"`
}

type MultiTimeframeData struct {
	Daily  []RecentBar `json:"daily"`
	Hourly []RecentBar `json:"hourly"`
}

type PortfolioState struct {
	CurrentPosition int     `json:"current_position"`
	AvgCost         float64 `json:"avg_cost"`
	MaxLimit        int     `json:"max_position_limit"`
	AccountBalance  float64 `json:"account_balance"`
}

type AnalysisGuidelines struct {
	DataDependency    string `json:"data_dependency"`
	NoFabrication     string `json:"no_fabrication"`
	UncertaintyExpr   string `json:"uncertainty_expression"`
	ReasoningFormat   string `json:"reasoning_format"`
	AvoidGeneralities string `json:"avoid_generalities"`
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
	// 拉取多周期历史 K 线
	dailyBars, err := cb.dataProvider.FetchHistoricalBars(tick.Symbol, "1d", 70)
	if err != nil {
		return AIContext{}, fmt.Errorf("failed to fetch daily bars: %w", err)
	}
	hourlyBars, err := cb.dataProvider.FetchHistoricalBars(tick.Symbol, "1h", 200)
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
	b, err := json.MarshalIndent(ctx, "", "  ")
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
