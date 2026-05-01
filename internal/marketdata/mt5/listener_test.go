package mt5

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/wancm/trader-bot/internal/marketdata"
)

func TestListenerDecodesTickStream(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	const expected = 3
	var (
		mu   sync.Mutex
		got  []marketdata.Tick
		done = make(chan struct{})
	)

	listener := &Listener{
		OnTick: func(tk marketdata.Tick) {
			mu.Lock()
			got = append(got, tk)
			n := len(got)
			mu.Unlock()
			if n == expected {
				close(done)
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- listener.Serve(ctx, ln) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	payload := `{"symbol":"EURUSD","bid":1.085,"ask":1.0852,"volume":123,"rsi":45.2,"timestamp":1741305600}` + "\n" +
		`{"symbol":"AAPL","bid":178.5,"ask":178.55,"volume":1000,"rsi":28.5,"timestamp":1741305600}` + "\n" +
		`{"symbol":"GOOG","bid":2800.1,"ask":2800.2,"volume":50,"rsi":51,"timestamp":1741305602}` + "\n"
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		mu.Lock()
		t.Fatalf("expected %d ticks, got %d: %+v", expected, len(got), got)
	}

	mu.Lock()
	defer mu.Unlock()

	if got[0].Symbol != "EURUSD" || got[0].Bid != 1.085 || got[0].Timestamp != 1741305600 {
		t.Errorf("first tick wrong: %+v", got[0])
	}
	if got[1].Symbol != "AAPL" || got[1].Ask != 178.55 {
		t.Errorf("second tick wrong: %+v", got[1])
	}
	if got[2].Symbol != "GOOG" || got[2].Volume != 50 || got[2].Timestamp != 1741305602 {
		t.Errorf("third tick wrong: %+v", got[2])
	}

	cancel()
	select {
	case <-serveErr:
	case <-time.After(time.Second):
		t.Error("Serve did not return after context cancel")
	}
}
