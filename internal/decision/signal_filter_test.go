// filter_test.go
package decision

import (
	"testing"
	"time"

	"github.com/wancm/trader-bot/internal/marketdata"
)

func TestRSIRule_Oversold(t *testing.T) {
	rule := RSIRule{Threshold: 30, Above: false, CooldownDuration: 1 * time.Minute}
	tick := marketdata.Tick{Symbol: "XAUUSD", RSI: 28.5}
	ok, reason := rule.Evaluate(tick)
	if !ok {
		t.Fatal("expected rule to trigger for RSI 28.5")
	}
	t.Log(reason)
}

func TestRSIRule_NotTriggered(t *testing.T) {
	rule := RSIRule{Threshold: 30, Above: false, CooldownDuration: 1 * time.Minute}
	tick := marketdata.Tick{Symbol: "XAUUSD", RSI: 35.0}
	ok, _ := rule.Evaluate(tick)
	if ok {
		t.Fatal("expected rule NOT to trigger for RSI 35.0")
	}
}

func TestSignalFilter_Cooldown(t *testing.T) {
	signalChan := make(chan SignalEvent, 10)
	rules := []Rule{
		RSIRule{Threshold: 30, Above: false, CooldownDuration: 500 * time.Millisecond},
	}
	filter := NewSignalFilter(rules, signalChan)

	tick := marketdata.Tick{Symbol: "XAUUSD", RSI: 28.0}

	// 第一次应该触发
	filter.ProcessTick(tick)
	select {
	case ev := <-signalChan:
		t.Logf("first trigger: %v", ev)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected signal event")
	}

	// 立即再发一次，应在冷却期内被拦截
	filter.ProcessTick(tick)
	select {
	case <-signalChan:
		t.Fatal("should not trigger again during cooldown")
	case <-time.After(100 * time.Millisecond):
		// 预期没有信号
	}

	// 等待冷却时间后应再次触发
	time.Sleep(600 * time.Millisecond)
	filter.ProcessTick(tick)
	select {
	case ev := <-signalChan:
		t.Logf("second trigger after cooldown: %v", ev)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected signal event after cooldown")
	}
}
