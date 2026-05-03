package broker

import (
	"context"
)

// OrderRequest 是标准化下单请求
type OrderRequest struct {
	Symbol    string
	Action    string  // BUY, SELL
	Quantity  float64 // 手数（外汇/黄金）或股数
	OrderType string  // MARKET, LIMIT
	Price     float64 // 限价单价格（市价单忽略）
	Reason    string  // 决策原因
}

// OrderResponse 是下单后返回的结果
type OrderResponse struct {
	OrderID      int64
	Status       string // FILLED, PENDING, REJECTED
	FilledQty    float64
	AvgFillPrice float64
	Commission   float64
	Error        string
}

// Position 持仓信息
type Position struct {
	Symbol   string
	Quantity float64
	AvgPrice float64
}

// Account 账户信息
type Account struct {
	Balance float64
	Equity  float64
}

// Broker 统一券商接口
type Broker interface {
	PlaceOrder(ctx context.Context, req OrderRequest) (OrderResponse, error)
	GetAccount(ctx context.Context) (Account, error)
	GetPositions(ctx context.Context) ([]Position, error)
	// 后续可扩展 CancelOrder 等
}
