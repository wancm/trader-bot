package shared

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// wsLogEntry matches the logger service ingest schema.
type wsLogEntry struct {
	SourceSystem string `json:"source_system,omitempty"`
	Action       string `json:"action"`
	Timestamp    string `json:"timestamp"`
	Type         string `json:"type"`
	Message      string `json:"message"`
	Tick         any    `json:"tick,omitempty"`
}

// wsHandler tees each slog record to both a console handler and a wsSender.
type wsHandler struct {
	inner  slog.Handler
	sender *wsSender
}

func (h *wsHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *wsHandler) Handle(ctx context.Context, r slog.Record) error {
	h.sender.enqueue(r)
	return h.inner.Handle(ctx, r)
}

func (h *wsHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &wsHandler{inner: h.inner.WithAttrs(attrs), sender: h.sender}
}

func (h *wsHandler) WithGroup(name string) slog.Handler {
	return &wsHandler{inner: h.inner.WithGroup(name), sender: h.sender}
}

// wsSender asynchronously forwards log entries to the logger service.
type wsSender struct {
	wsURL        string
	sourceSystem string
	queue        chan wsLogEntry
}

func newWSSender(wsURL, sourceSystem string) *wsSender {
	return &wsSender{
		wsURL:        wsURL,
		sourceSystem: sourceSystem,
		queue:        make(chan wsLogEntry, 512),
	}
}

// send pushes an entry non-blocking. Drops silently if the queue is full.
func (s *wsSender) send(e wsLogEntry) {
	select {
	case s.queue <- e:
	default:
	}
}

// enqueue converts a slog.Record to a wsLogEntry and sends it.
func (s *wsSender) enqueue(r slog.Record) {
	s.send(wsLogEntry{
		SourceSystem: s.sourceSystem,
		Action:       "log",
		Timestamp:    r.Time.UTC().Format(time.RFC3339),
		Type:         levelToType(r.Level),
		Message:      r.Message,
	})
}

// Run connects to wsURL and drains the queue until ctx is cancelled.
// Waits 5 seconds before retrying on failure.
func (s *wsSender) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.dial(ctx); err != nil && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (s *wsSender) dial(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, s.wsURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[logger-ws] connect failed %s: %v\n", s.wsURL, err)
		return err
	}
	fmt.Fprintf(os.Stderr, "[logger-ws] connected to %s\n", s.wsURL)
	defer func() {
		fmt.Fprintf(os.Stderr, "[logger-ws] disconnected from %s\n", s.wsURL)
		conn.CloseNow()
	}()

	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "")
			return nil
		case e := <-s.queue:
			writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := wsjson.Write(writeCtx, conn, e)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func levelToType(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "error"
	case level >= slog.LevelWarn:
		return "warning"
	case level >= slog.LevelInfo:
		return "info"
	default:
		return "verbose"
	}
}

// handlerOpts returns slog.HandlerOptions that emit timestamps in UTC.
func handlerOpts() *slog.HandlerOptions {
	return &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			}
			return a
		},
	}
}

// LogForwarder allows sending log entries directly to the logger service,
// bypassing slog level mapping. Use for entry types like "tick" that have
// no slog level equivalent.
type LogForwarder struct {
	sender *wsSender
}

// Run starts the background sender goroutine. Call after the signal context
// is created: go forwarder.Run(ctx).
func (f *LogForwarder) Run(ctx context.Context) { f.sender.Run(ctx) }

// SendEntry forwards an entry of the given type directly to the logger service.
// timestamp should be the event time (e.g. the tick's own timestamp).
// An optional tick payload can be passed as the last argument.
func (f *LogForwarder) SendEntry(typ, message string, timestamp time.Time, tick ...any) {
	e := wsLogEntry{
		SourceSystem: f.sender.sourceSystem,
		Action:       "log",
		Timestamp:    timestamp.UTC().Format(time.RFC3339),
		Type:         typ,
		Message:      message,
	}
	if len(tick) > 0 {
		e.Tick = tick[0]
	}
	f.sender.send(e)
}

// NewLoggerWithWS creates a logger that writes to the console AND forwards
// every record to the logger service at wsURL. Start the forwarder's Run
// method in a goroutine after the signal context is created.
func NewLoggerWithWS(format, wsURL, sourceSystem string) (*slog.Logger, *LogForwarder) {
	sender := newWSSender(wsURL, sourceSystem)
	opts := handlerOpts()
	var inner slog.Handler
	if format == "json" {
		inner = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		inner = slog.NewTextHandler(os.Stdout, opts)
	}
	l := slog.New(&wsHandler{inner: inner, sender: sender})
	slog.SetDefault(l)
	return l, &LogForwarder{sender: sender}
}
