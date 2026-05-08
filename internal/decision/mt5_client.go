// decision/mt5_client.go
package decision

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// HistoricalDataProvider 定义获取历史数据的接口，方便测试 mock
type HistoricalDataProvider interface {
	FetchHistoricalBars(symbol, timeframe string, count int) ([]MT5Bar, error)
}

// 确保 MT5Client 实现了该接口
var _ HistoricalDataProvider = (*MT5Client)(nil)

// MT5Bar 表示从 MT5 返回的一根历史 K 线
// 假设 mt5_remote 返回的 time 为 Unix 秒，我们用 int64 接收再转换
type MT5Bar struct {
	Timestamp int64   `json:"time"` // Unix 秒
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"tick_volume"`
}

// Time 返回转换后的 time.Time
func (b MT5Bar) Time() time.Time {
	return time.Unix(b.Timestamp, 0)
}

// MT5Client 通过 HTTP 从 mt5_remote 拉取历史数据
type MT5Client struct {
	BaseURL    string       // 例如 "http://localhost:18812"
	HTTPClient *http.Client // 自定义 HTTP 客户端（可选）
	logger     *slog.Logger
}

// NewMT5Client 创建 MT5 客户端
func NewMT5Client(baseURL string, logger *slog.Logger) *MT5Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &MT5Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// FetchHistoricalBars 请求历史 K 线
// symbol: 品种，如 "XAUUSD"
// timeframe: 周期，如 "5m", "1h", "1d"
// count: 拉取的根数
func (c *MT5Client) FetchHistoricalBars(symbol, timeframe string, count int) ([]MT5Bar, error) {
	url := fmt.Sprintf("%s/history?symbol=%s&timeframe=%s&count=%d",
		c.BaseURL, symbol, timeframe, count)

	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("mt5 request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Error("mt5 returned error", "symbol", symbol, "status", resp.StatusCode, "err", string(body))
		return nil, fmt.Errorf("mt5 returned %d: %s", resp.StatusCode, string(body))
	}

	var bars []MT5Bar
	if err := json.NewDecoder(resp.Body).Decode(&bars); err != nil {
		return nil, fmt.Errorf("failed to decode mt5 response: %w", err)
	}

	// c.logger.Info("mt5 returned data", "symbol", symbol, "bars", bars)

	return bars, nil
}
