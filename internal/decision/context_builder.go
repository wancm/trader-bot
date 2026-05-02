package decision

import (
	"encoding/json"
)

// AIContext 是发送给 DeepSeek 的完整 JSON 结构
type AIContext struct {
	TraderProfile       TraderProfile       `json:"trader_profile"`
	MarketSnapshot      MarketSnapshot      `json:"market_snapshot"`
	TechnicalIndicators TechnicalIndicators `json:"technical_indicators"`
	RecentPriceAction   RecentPriceAction   `json:"recent_price_action"`
	PortfolioState      PortfolioState      `json:"portfolio_state"`
	AnalysisGuidelines  AnalysisGuidelines  `json:"analysis_guidelines"`
}

// 子结构体定义（简化版，后续可按需扩展）
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
	// 其他字段可补充
}
type TechnicalIndicators struct {
	RSI float64 `json:"rsi_14"`
	// 未来可加 MACD, SMA 等
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

// ContextBuilder 构建 AI 上下文
type ContextBuilder struct {
	aggregator *BarAggregator
	// 未来可增加 PortfolioManager 客户端
}

func NewContextBuilder(aggregator *BarAggregator) *ContextBuilder {
	return &ContextBuilder{aggregator: aggregator}
}

// BuildContext 根据最新的 Tick 构建 AI 上下文
func (cb *ContextBuilder) BuildContext(tick TickData, portfolio PortfolioState) (AIContext, error) {
	bars := cb.aggregator.GetRecentBars(20) // 取最近 20 根闭合 Bar
	recentBars := make([]RecentBar, len(bars))
	for i, b := range bars {
		recentBars[i] = RecentBar{
			Open:  b.Open,
			High:  b.High,
			Low:   b.Low,
			Close: b.Close,
		}
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
			Timeframe: "5m",
			Bars:      recentBars,
		},
		PortfolioState: portfolio,
		AnalysisGuidelines: AnalysisGuidelines{
			DataDependency:    "Base all observations strictly on provided data.",
			NoFabrication:     "Do not invent any values or patterns not present.",
			UncertaintyExpr:   "If uncertain, set confidence < 0.5 and recommend HOLD.",
			ReasoningFormat:   "Step-by-step referencing specific numeric values.",
			AvoidGeneralities: "Avoid vague statements, always cite exact price/indicator values.",
		},
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
