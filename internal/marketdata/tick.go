// Package marketdata defines the unified market-data types consumed by trader-bot.
package marketdata

// Tick is one market quote snapshot.
//
// The JSON tags must stay byte-compatible with the line written by the
// mt5-bridge DLL ([applications/mt5-bridge/Mt5Bridge.Core/TickFormatter.cpp]):
//
//	{"symbol":"EURUSD","bid":1.0850,"ask":1.0852,"volume":123,"rsi":45.2,"timestamp":1741305600}\n
type Tick struct {
	Symbol    string  `json:"symbol"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	Volume    float64 `json:"volume"`
	RSI       float64 `json:"rsi"`
	Timestamp int64   `json:"timestamp"`
}
