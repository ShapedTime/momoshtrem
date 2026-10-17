package observe

import (
	"strings"
	"sync"
	"testing"
	"time"
)

const mutexText = `--- mutex:
cycles/second=1000000000
sampling period=10
3000000 4 @ 0x1 0x2 0x3 0x4
#	0x1	sync.(*Mutex).Unlock+0x4f	/go/src/sync/mutex.go:229
#	0x2	github.com/anacrolix/torrent.(*lockWithDeferreds).Unlock+0x20	/mod/deferrwl.go:40
#	0x3	github.com/anacrolix/torrent.(*Piece).SetPriority+0x90	/mod/piece.go:272
#	0x4	github.com/shapedtime/momoshtrem/internal/streaming.(*Prioritizer).setPieceRangePriorityCount+0x10	/src/prioritizer.go:291

1000000 1 @ 0x5 0x6
#	0x5	sync.(*Mutex).Unlock+0x4f	/go/src/sync/mutex.go:229
#	0x6	github.com/shapedtime/momoshtrem/internal/torrent/piececache.(*Cache).Remove+0x10	/src/cache.go:10

500000 2 @ 0x7
#	0x7	runtime.unlock2+0x10	/go/src/runtime/lock.go:10
`

func TestParseContention(t *testing.T) {
	recs := parseContention(mutexText)
	if len(recs) != 3 {
		t.Fatalf("records = %+v", recs)
	}
	r := recs[0]
	if r.count != 4 || r.delayMS != 3 || !r.clientLock || !r.setPriority || !r.ownCode {
		t.Errorf("first record = %+v", r)
	}
	if site, callers, ok := r.site(); !ok || site != "github.com/anacrolix/torrent.(*Piece).SetPriority" || len(callers) != 1 {
		t.Errorf("site = %q %v %v", site, callers, ok)
	}
	if _, _, ok := recs[2].site(); ok {
		t.Error("a record of runtime frames only has a site")
	}
	sites := top(recs, func(contentionRecord) bool { return true })
	if len(sites) != 2 || sites[0].DelayMS != 3 || !strings.Contains(sites[0].Note, "SetPriority (F7)") ||
		!strings.Contains(sites[0].Note, "anacrolix client lock") {
		t.Errorf("sites = %+v", sites)
	}
}

// The real profile parses: contention made here shows at its site.
func TestCollectProfiles(t *testing.T) {
	p := startProfiles(true)
	defer p.stop()
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				mu.Lock()
				time.Sleep(10 * time.Microsecond)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	c := collectProfiles(t.TempDir())
	if c.artifacts["mutex-profile"] != "mutex.pprof" || c.artifacts["block-profile"] != "block.pprof" {
		t.Errorf("artifacts = %v", c.artifacts)
	}
	found := false
	for _, s := range c.mutex {
		found = found || strings.Contains(s.Site, "TestCollectProfiles")
	}
	if !found {
		t.Errorf("the test's contention isn't among the sites: %+v", c.mutex)
	}
}
