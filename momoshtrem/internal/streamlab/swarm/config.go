// Package swarm runs streamlab's synthetic peers (docs/streamlab/PLAN.md
// §5): anacrolix seeders whose pieces are generated from a media.Layout
// (paced, partial, or serving a corrupt piece), on loopback, found through
// the magnet's x.pe peer address, directly or through a Proxy that shapes
// the link (latency, jitter, bandwidth) and cuts it (Drop, Vanish).
package swarm

import (
	"context"
	"log/slog"

	tlog "github.com/anacrolix/log"
	atorrent "github.com/anacrolix/torrent"
)

// loopbackHost is where every lab client listens.
const loopbackHost = "127.0.0.1"

// Loopback applies the lab's overrides to a client config, and nothing
// else (PLAN D3): listen on 127.0.0.1 on a free port, no DHT, trackers or
// port forwarding, no accept rate limiting, TCP only, and a lab logger.
// The seeders use it, and so does a leecher built on production's config.
//
// Production runs anacrolix's pure-Go uTP; over loopback the lab runs TCP
// by choice. It never uses atorrent.TestingConfig, whose
// MaxAllocPeerRequestDataPerConn and KeepAliveTimeout are far from
// production's.
func Loopback(cfg *atorrent.ClientConfig, name string) {
	cfg.ListenHost = func(string) string { return loopbackHost }
	cfg.ListenPort = 0
	cfg.NoDHT = true
	cfg.DisableTrackers = true
	cfg.NoDefaultPortForwarding = true
	cfg.DisableAcceptRateLimiting = true
	cfg.DisableUTP = true
	cfg.Logger = Logger(name)
}

// Logger is an anacrolix logger that sends warnings and errors to slog
// with component=name, as production's client logger does. It keeps
// anacrolix's default filter, so debug and info records are dropped.
// Unlike production's, it keeps the attributes of anacrolix's slog records
// (the piece, the error), which say what went wrong.
func Logger(name string) tlog.Logger {
	l := tlog.NewLogger(name)
	l.SetHandlers(slogHandler{log: slog.With("component", name)})
	return l
}

// slogHandler maps anacrolix log records onto slog levels.
type slogHandler struct {
	log *slog.Logger
}

func (h slogHandler) Handle(r tlog.Record) {
	level := slog.LevelDebug
	switch r.Level {
	case tlog.Critical, tlog.Error:
		level = slog.LevelError
	case tlog.Warning:
		level = slog.LevelWarn
	case tlog.Info:
		level = slog.LevelInfo
	}
	var attrs []any
	if rec := r.Msg.SlogRecord(); rec.Ok {
		rec.Value.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, a)
			return true
		})
	}
	h.log.Log(context.Background(), level, r.Msg.String(), attrs...)
}
