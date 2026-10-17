package scenario

import (
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
)

// #225's signature: nothing left but timed-out reads' goroutines blocked
// in anacrolix, after a read timed out.
func TestOrphanedReads(t *testing.T) {
	orphan := checks.LabelledGoroutines{Request: "s1.2", Count: 1, Stack: `# labels: {"slreq":"s1.2"}
#	0x1	github.com/anacrolix/torrent.(*reader).waitAvailable+0x1c3	reader.go:242
#	0x2	github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).Read+0x177	reader.go:143
#	0x3	github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1+0x3c	torrent_file.go:470`}
	other := checks.LabelledGoroutines{Request: "s1.3", Count: 1, Stack: `#	0x1	github.com/shapedtime/momoshtrem/internal/webdav.(*webdavFile).Read+0x1	server.go:1`}
	tests := []struct {
		name     string
		q        checks.QuiescenceResult
		timeouts float64
		want     string
	}{
		{"orphans after timeouts", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{orphan, orphan}}, 2, "#225"},
		{"more orphans than timeouts", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{orphan, orphan}}, 1, ""},
		{"6 orphans after 1 timeout", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{{Request: "s1.2", Count: 6, Stack: orphan.Stack}}}, 1, ""},
		{"no read timed out", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{orphan}}, 0, ""},
		{"another leak too", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{orphan, other}}, 1, ""},
		{"a request still running", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{orphan}, Requests: 1}, 1, ""},
		{"a reader left", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{orphan}, Readers: 1}, 1, ""},
		{"sessions left, no leak", checks.QuiescenceResult{Sessions: 1}, 1, ""},
	}
	for _, tt := range tests {
		if got := orphanedReads(tt.q, tt.timeouts); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// play-ok names every known issue its failures carry, each by its own
// signature; one unexplained failure, a #195 stall on more than two
// pieces, or more #195 failures than read timeouts leaves it unexplained.
func TestPlayKnown(t *testing.T) {
	title := &Title{Hash: "aa"}
	stall := func(piece int) failure {
		return failure{title: title, piece: piece, timedOut: true, stalled: true, broke: true}
	}
	evicted := func(piece int) failure {
		return failure{title: title, piece: piece, silence: 10 * time.Millisecond, broke: true, evicted: true, wasComplete: true}
	}
	plain := failure{title: title, piece: 9, broke: true}
	tests := []struct {
		name     string
		fs       []failure
		timeouts float64
		want     string
	}{
		{"one stall", []failure{stall(3), stall(3)}, 2, "#195"},
		{"a stall with fewer read timeouts", []failure{stall(3), stall(3)}, 1, ""},
		{"stalls on three pieces", []failure{stall(1), stall(2), stall(3)}, 3, ""},
		{"evicted pieces", []failure{evicted(1), evicted(5), evicted(9)}, 0, "F11"},
		{"both, each its own", []failure{evicted(1), stall(3)}, 1, "#195+F11"},
		{"an unexplained one among them", []failure{evicted(1), plain}, 0, ""},
		{"a partial piece's break isn't F11", []failure{{title: title, piece: 4, silence: time.Millisecond, broke: true, evicted: true}}, 0, ""},
	}
	for _, tt := range tests {
		if got := playKnown(tt.fs, tt.timeouts); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}
