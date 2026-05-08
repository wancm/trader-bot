package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	actionBuy      = "BUY"
	actionSell     = "SELL"
	orderMarket    = "MARKET"
	orderLimit     = "LIMIT"
	statusFilled   = "FILLED"
	statusPending  = "PENDING"
	statusRejected = "REJECTED"
)

type PaperBroker struct {
	mu   sync.RWMutex
	pool *pgxpool.Pool
	// 最新报价（外部注入）
	latestBid float64
	latestAsk float64
}

// NewPaperBroker 创建一个模拟券商
func NewPaperBroker(pool *pgxpool.Pool) *PaperBroker {
	return &PaperBroker{pool: pool}
}

// UpdatePrices 更新当前报价（由行情回调驱动）
func (b *PaperBroker) UpdatePrices(_ string, bid, ask float64) {
	// 简化处理，只记最新值；后续可根据 symbol 区分
	b.mu.Lock()
	b.latestBid, b.latestAsk = bid, ask
	b.mu.Unlock()
}

// PlaceOrder 下单入口
func (b *PaperBroker) PlaceOrder(ctx context.Context, req OrderRequest) (OrderResponse, error) {
	// 1. 插入订单，状态 PENDING
	var orderID int64
	err := b.pool.QueryRow(ctx,
		`INSERT INTO broker_orders (symbol, action, order_type, quantity, limit_price, reason)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		req.Symbol, req.Action, req.OrderType, req.Quantity, nullableFloat(req.Price, req.OrderType == orderLimit), req.Reason,
	).Scan(&orderID)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("insert order: %w", err)
	}

	switch req.OrderType {
	case orderMarket:
		// 立即按市价成交
		return b.fillMarketOrder(ctx, orderID, req)
	case orderLimit:
		// 限价单暂不成交，等待后续检查；此处直接返回 PENDING
		return OrderResponse{OrderID: orderID, Status: statusPending}, nil
	default:
		return OrderResponse{OrderID: orderID, Status: statusRejected, Error: "unsupported order type"}, nil
	}
}

// fillMarketOrder 市价成交
func (b *PaperBroker) fillMarketOrder(ctx context.Context, orderID int64, req OrderRequest) (OrderResponse, error) {
	// 获取费用参数
	fee, err := b.loadFeeConfig(ctx)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("load fee config: %w", err)
	}

	b.mu.RLock()
	bid, ask := b.latestBid, b.latestAsk
	b.mu.RUnlock()

	// 成交价：买入用 ask，卖出用 bid，并加滑点
	var fillPrice float64
	if req.Action == actionBuy {
		fillPrice = ask + fee.slippageValue()
	} else {
		fillPrice = bid - fee.slippageValue()
	}
	if fillPrice <= 0 {
		return OrderResponse{OrderID: orderID, Status: statusRejected, Error: "no market price available"}, nil
	}

	// 佣金计算：每手固定 + 交易额百分比
	commission := fee.commissionPerLot*req.Quantity + (fillPrice*req.Quantity*100)*fee.commissionPct
	// 注意：此处假设一手合约规模为100单位（如黄金100盎司），如果是外汇标准手100,000，需要调整。简化处理。

	// 更新订单状态
	_, err = b.pool.Exec(ctx,
		`UPDATE broker_orders SET status=$1, filled_qty=$2, avg_fill_price=$3, commission=$4, updated_at=NOW() WHERE id=$5`,
		statusFilled, req.Quantity, fillPrice, commission, orderID)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("update order: %w", err)
	}

	// 插入成交记录
	_, err = b.pool.Exec(ctx,
		`INSERT INTO broker_trades (order_id, symbol, action, quantity, price, commission) VALUES ($1,$2,$3,$4,$5,$6)`,
		orderID, req.Symbol, req.Action, req.Quantity, fillPrice, commission)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("insert trade: %w", err)
	}

	// 更新持仓
	var delta float64
	if req.Action == actionBuy {
		delta = req.Quantity
	} else {
		delta = -req.Quantity
	}
	_, err = b.pool.Exec(ctx,
		`INSERT INTO broker_positions (symbol, quantity, avg_price)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (symbol) DO UPDATE SET
		     quantity = broker_positions.quantity + EXCLUDED.quantity,
		     avg_price = CASE WHEN broker_positions.quantity + EXCLUDED.quantity = 0 THEN 0
		                     ELSE (broker_positions.avg_price * broker_positions.quantity + $3 * $2) / (broker_positions.quantity + EXCLUDED.quantity)
		                END,
		     updated_at = NOW()`,
		req.Symbol, delta, fillPrice)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("update position: %w", err)
	}

	// 更新账户余额（扣除成交金额+佣金） 买入扣钱，卖出加钱
	var cashChange float64
	if req.Action == actionBuy {
		cashChange = -(fillPrice*req.Quantity*100 + commission) // 假设1手=100单位
	} else {
		cashChange = fillPrice*req.Quantity*100 - commission
	}
	_, err = b.pool.Exec(ctx,
		`UPDATE broker_account SET balance = balance + $1, equity = balance + $1, updated_at = NOW() WHERE id=1`,
		cashChange)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("update account: %w", err)
	}

	return OrderResponse{
		OrderID:      orderID,
		Status:       statusFilled,
		FilledQty:    req.Quantity,
		AvgFillPrice: fillPrice,
		Commission:   commission,
	}, nil
}

// GetAccount 返回当前账户信息
func (b *PaperBroker) GetAccount(ctx context.Context) (Account, error) {
	var acc Account
	if err := b.pool.QueryRow(ctx, `SELECT balance, equity FROM broker_account WHERE id=1`).Scan(&acc.Balance, &acc.Equity); err != nil {
		return acc, fmt.Errorf("get account: %w", err)
	}
	return acc, nil
}

// GetPositions 返回所有持仓
func (b *PaperBroker) GetPositions(ctx context.Context) ([]Position, error) {
	rows, err := b.pool.Query(ctx, `SELECT symbol, quantity, avg_price FROM broker_positions WHERE quantity != 0`)
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}
	defer rows.Close()
	var pos []Position
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.Symbol, &p.Quantity, &p.AvgPrice); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		pos = append(pos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}
	return pos, nil
}

// -------------------- 费用工具 --------------------

type feeConfig struct {
	commissionPerLot float64
	commissionPct    float64
	spreadPoints     float64
	slippagePoints   float64
	swapLong         float64
	swapShort        float64
}

func (f feeConfig) slippageValue() float64 {
	// 1 点 = 0.01（黄金）或 0.00001（外汇），这里简化用 0.01
	return f.slippagePoints * 0.01
}

func (b *PaperBroker) loadFeeConfig(ctx context.Context) (feeConfig, error) {
	var f feeConfig
	err := b.pool.QueryRow(ctx,
		`SELECT commission_per_lot, commission_pct, spread_points, slippage_points, swap_long_points, swap_short_points FROM broker_fee_config WHERE id=1`,
	).Scan(&f.commissionPerLot, &f.commissionPct, &f.spreadPoints, &f.slippagePoints, &f.swapLong, &f.swapShort)
	if err != nil {
		return feeConfig{}, fmt.Errorf("load fee config: %w", err)
	}
	return f, nil
}

// nullableFloat 辅助函数：限价单才需要价格，市价单忽略
func nullableFloat(price float64, need bool) interface{} {
	if need {
		return price
	}
	return nil
}
