package checks

import (
	"context"
	"runtime/pprof"
	"strings"
	"testing"
)

// A goroutine started under the label, running momoshtrem code, is found
// with its request; one of another stack is ignored.
func TestLabelledGoroutines_FindsOwnCode(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	started := make(chan struct{}, 2)
	for _, id := range []string{"s9.1", "s10.1"} {
		pprof.Do(context.Background(), pprof.Labels("sltest", id), func(context.Context) {
			go func() {
				started <- struct{}{}
				<-stop
			}()
		})
	}
	<-started
	<-started

	own, others := labelledGoroutines("sltest", func(id string) bool { return strings.HasPrefix(id, "s9.") })
	if len(own) != 1 || own[0].Request != "s9.1" || own[0].Count != 1 || others != 0 {
		t.Fatalf("own = %+v, others = %d; want s9.1's goroutine alone", own, others)
	}
	if !strings.Contains(own[0].Stack, "TestLabelledGoroutines_FindsOwnCode") {
		t.Errorf("stack doesn't name the goroutine's function:\n%s", own[0].Stack)
	}
}

// The parser reads counts and labels, skips unlabelled records, and tells
// library goroutines from momoshtrem's by their frames.
func TestParseLabelled(t *testing.T) {
	profile := `goroutine profile: total 9
3 @ 0x1 0x2
# labels: {"slreq":"s1.4"}
#	0x1	github.com/anacrolix/torrent.(*Client).outgoingConnection+0x1	/x/client.go:1

2 @ 0x3 0x4
# labels: {"other":"x", "slreq":"s1.7"}
#	0x3	github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1+0x1	/x/torrent_file.go:1

1 @ 0x5
#	0x5	github.com/shapedtime/momoshtrem/internal/torrent.(*ActivityManager).idleCheckLoop+0x1	/x/activity.go:1

3 @ 0x6
# labels: {"slreq":"s2.1"}
#	0x6	github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1+0x1	/x/torrent_file.go:1

`
	own, others := parseLabelled(profile, "slreq", func(id string) bool { return strings.HasPrefix(id, "s1.") })
	if others != 3 {
		t.Errorf("others = %d, want 3", others)
	}
	if len(own) != 1 || own[0].Request != "s1.7" || own[0].Count != 2 || !strings.Contains(own[0].Stack, "readContext") {
		t.Errorf("own = %+v, want s1.7's two reads", own)
	}
}
