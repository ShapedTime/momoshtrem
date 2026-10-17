package scenario

import (
	"testing"

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
		{"orphans after a timeout", checks.QuiescenceResult{Leaked: []checks.LabelledGoroutines{orphan, orphan}}, 1, "#225"},
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
