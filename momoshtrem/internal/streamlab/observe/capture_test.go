package observe

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newCapture(diagnose bool) (*Capture, *bytes.Buffer) {
	var out bytes.Buffer
	return NewCapture(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelInfo}), diagnose), &out
}

// Measure mode records WARN and above only, and a DEBUG call is disabled
// as it is without the capture; the wrapped handler still gets INFO.
func TestCaptureMeasureMode(t *testing.T) {
	c, out := newCapture(false)
	l := slog.New(c)
	prio := l.With("component", "prioritizer", "file", "Film.mkv")
	if prio.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("DEBUG is enabled in measure mode")
	}
	prio.Debug("updated seek priorities", "offset", 0)
	l.Info("torrent client created", "component", "torrent-client")
	l.With("component", "streamlab-leecher").Error("error reading", "piece", 3)
	evs := c.Events(time.Time{})
	if len(evs) != 1 || evs[0].Msg != "error reading" || evs[0].Component != "streamlab-leecher" || evs[0].Get("piece") != "3" {
		t.Fatalf("events = %+v, want the ERROR only", evs)
	}
	if !strings.Contains(out.String(), "torrent client created") || strings.Contains(out.String(), "updated seek priorities") {
		t.Errorf("the wrapped handler got:\n%s", out.String())
	}
}

// --diagnose records the streaming components' DEBUG records, not
// others', and doesn't pass them on to an INFO handler.
func TestCaptureDiagnose(t *testing.T) {
	c, out := newCapture(true)
	l := slog.New(c)
	l.With("component", "prioritizer", "file", "Film.mkv").Debug("updated seek priorities", "offset", 0)
	l.With("component", "activity-manager").Debug("torrent idle")
	l.With("component", "torrent-service").Debug("noise")
	evs := c.Events(time.Time{})
	if len(evs) != 2 || evs[0].Get("file") != "Film.mkv" || evs[0].Get("offset") != "0" || evs[1].Component != "activity-manager" {
		t.Fatalf("events = %+v", evs)
	}
	if out.Len() != 0 {
		t.Errorf("DEBUG reached the INFO handler:\n%s", out.String())
	}
}

// anacrolix's read path, as its slog bridge logs it: a torrent group, then
// the failed read's piece.
func leecher(c *Capture, ih string) *slog.Logger {
	return slog.New(c).With("component", "streamlab-leecher").With(slog.Group("torrent", "name", "Film.mkv", "ih", ih))
}

func failRead(l *slog.Logger, piece int, resync bool) {
	l.Error(MsgReadError, "piece", piece, "offset", 0, "err", "EOF")
	l.Error(MsgInitialFailed, "err", "EOF")
	if !resync {
		return
	}
	l.Error(MsgReadError, "piece", piece, "offset", 0, "err", "EOF")
	l.Error(MsgResetFailed, "err", "EOF")
	l.Error(MsgReadError, "piece", piece, "offset", 0, "err", "EOF")
	l.Error(MsgResyncFailed, "err", "EOF")
}

// F11's log proof: the read of that piece of that torrent failed after
// the completion resync, just before the break.
func TestReadFailed(t *testing.T) {
	c, _ := newCapture(false)
	failRead(leecher(c, "aaaa"), 7, true)
	failRead(leecher(c, "aaaa"), 9, false) // recovered: #219's noise
	now := time.Now()
	tests := []struct {
		name  string
		ih    string
		piece int
		at    time.Time
		want  bool
	}{
		{"the piece", "aaaa", 7, now, true},
		{"the hash in capitals", "AAAA", 7, now, true},
		{"another piece", "aaaa", 8, now, false},
		{"a read that recovered", "aaaa", 9, now, false},
		{"another torrent", "bbbb", 7, now, false},
		{"long after", "aaaa", 7, now.Add(proofWindow + time.Second), false},
		{"before it", "aaaa", 7, now.Add(-2 * time.Second), false},
	}
	for _, tt := range tests {
		if got := c.ReadFailed(tt.ih, tt.piece, tt.at); got != tt.want {
			t.Errorf("%s: ReadFailed = %v, want %v", tt.name, got, tt.want)
		}
	}
	var nilCapture *Capture
	if nilCapture.ReadFailed("aaaa", 7, now) {
		t.Error("no capture proved something")
	}

	// The other order: piece 9's read recovers, then piece 7's gives up.
	// Only the giving-up read's own last "error reading" proves its piece.
	r, _ := newCapture(false)
	failRead(leecher(r, "aaaa"), 9, false)
	failRead(leecher(r, "aaaa"), 7, true)
	if r.ReadFailed("aaaa", 9, time.Now()) {
		t.Error("piece 9's recovered read was proved by piece 7's failure")
	}
	if !r.ReadFailed("aaaa", 7, time.Now()) {
		t.Error("piece 7's failure isn't proved")
	}
	// A record without the torrent's hash proves nothing.
	u, _ := newCapture(false)
	failRead(slog.New(u).With("component", "streamlab-leecher"), 7, true)
	if u.ReadFailed("aaaa", 7, time.Now()) {
		t.Error("records without torrent.ih proved F11")
	}
	s := summarizeLogs(c.Events(time.Time{}), c)
	if s.BenignStorageErrors != 1 || s.ReadFailures != 1 || s.Errors != 8 {
		t.Errorf("summary = %+v, want 1 benign, 1 failure, 8 errors", s)
	}
}
