package shared

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// TickForwarder streams market ticks to an external WebSocket endpoint.
// Reconnects automatically on failure.
type TickForwarder struct {
	wsURL string
	queue chan any
}

// NewTickForwarder creates a TickForwarder targeting wsURL.
func NewTickForwarder(wsURL string) *TickForwarder {
	return &TickForwarder{
		wsURL: wsURL,
		queue: make(chan any, 256),
	}
}

// Send enqueues v for forwarding. Drops silently if the queue is full.
func (f *TickForwarder) Send(v any) {
	select {
	case f.queue <- v:
	default:
	}
}

// Run connects to wsURL and drains the queue until ctx is cancelled.
// Waits 5 seconds before retrying on failure.
func (f *TickForwarder) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := f.dial(ctx); err != nil && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (f *TickForwarder) dial(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, f.wsURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[tick-fwd] connect failed %s: %v\n", f.wsURL, err)
		return err
	}
	fmt.Fprintf(os.Stderr, "[tick-fwd] connected to %s\n", f.wsURL)
	defer func() {
		fmt.Fprintf(os.Stderr, "[tick-fwd] disconnected from %s\n", f.wsURL)
		conn.CloseNow()
	}()

	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return nil
		case v := <-f.queue:
			writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := wsjson.Write(writeCtx, conn, v)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}
