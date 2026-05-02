// decision/engine.go
package decision

type Engine struct {
	signalFilter *SignalFilter
	// ... 未来：contextBuilder, aiClient, postProcessor, orderEmitter
}

func NewEngine(rules []Rule, signalChan chan SignalEvent) *Engine {
	return &Engine{
		signalFilter: NewSignalFilter(rules, signalChan),
	}
}

// ProcessTick 是 Facade 的唯一对外入口，由行情回调调用
func (e *Engine) ProcessTick(tick TickData) {
	e.signalFilter.ProcessTick(tick)
}
