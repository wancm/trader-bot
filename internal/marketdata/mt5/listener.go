// Package mt5 implements trader-bot's ingestion of the mt5-bridge tick stream:
// a TCP server that accepts persistent connections from Mt5Bridge.dll and
// decodes one newline-delimited JSON tick per record.
package mt5

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/wancm/trader-bot/internal/marketdata"
)

type Listener struct {
	Logger *slog.Logger
	OnTick func(marketdata.Tick)
}

func (l *Listener) ListenAndServe(ctx context.Context, addr string) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return l.Serve(ctx, ln)
}

func (l *Listener) Serve(ctx context.Context, ln net.Listener) error {
	log := l.logger()
	log.Info("mt5 tick listener bound", "addr", ln.Addr().String())

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go l.handleConn(ctx, conn)
	}
}

func (l *Listener) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	log := l.logger().With("remote", conn.RemoteAddr().String())
	log.Info("mt5 bridge connected")

	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	dec := json.NewDecoder(conn)
	var ticks uint64
	for {
		var t marketdata.Tick
		if err := dec.Decode(&t); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				log.Info("mt5 bridge disconnected", "ticks_received", ticks)
				return
			}
			log.Warn("decode error, closing connection", "err", err, "ticks_received", ticks)
			return
		}
		ticks++
		if l.OnTick != nil {
			l.OnTick(t)
			continue
		}
		// log.Info("tick",
		// 	"symbol", t.Symbol,
		// 	"bid", t.Bid,
		// 	"ask", t.Ask,
		// 	"volume", t.Volume,
		// 	"rsi", t.RSI,
		// 	"timestamp", t.Timestamp,
		// )
	}
}

func (l *Listener) logger() *slog.Logger {
	if l.Logger != nil {
		return l.Logger
	}
	return slog.Default()
}
