package shared

import "sync/atomic"

// Feature flags — package-level so any package can read/write them at runtime.
// Defaults are set in init(); the admin API toggles them without restart.
var (
	TickListening atomic.Bool // true: process and forward ticks
	MakeOrders    atomic.Bool // true: call AI and place orders
)

func init() {
	TickListening.Store(true)
	MakeOrders.Store(true)
}
