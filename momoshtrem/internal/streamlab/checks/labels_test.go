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

1 @ 0x7 0x8
# labels: {"slreq":"s1.9"}
#	0x7	runtime.gopark+0x1	/go/src/runtime/proc.go:1
#	0x8	time.Sleep+0x1	/go/src/runtime/time.go:1

`
	own, others := parseLabelled(profile, "slreq", func(id string) bool { return strings.HasPrefix(id, "s1.") })
	if others != 3 {
		t.Errorf("others = %d, want 3 (the anacrolix connection)", others)
	}
	if len(own) != 2 || own[0].Request != "s1.7" || own[0].Count != 2 || !strings.Contains(own[0].Stack, "readContext") ||
		own[1].Request != "s1.9" || own[1].Count != 1 {
		t.Errorf("own = %+v, want s1.7's two reads and s1.9's lone time.Sleep", own)
	}
}

func TestRootFunc(t *testing.T) {
	rec := "2 @ 0x1 0x2\n# labels: {\"slreq\":\"s1.1\"}\n" +
		"#\t0x1\tinternal/poll.runtime_pollWait+0x9f\t/go/src/runtime/netpoll.go:351\n" +
		"#\t0x2\tgithub.com/anacrolix/torrent.(*Client).outgoingConnection+0x5c\t/x/client.go:9"
	if got, want := rootFunc(rec), "github.com/anacrolix/torrent.(*Client).outgoingConnection+0x5c"; got != want {
		t.Errorf("rootFunc = %q, want %q", got, want)
	}
	if got := rootFunc("1 @ 0x1"); got != "" {
		t.Errorf("rootFunc of a record without frames = %q", got)
	}
}
