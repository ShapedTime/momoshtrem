package scenario

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
)

// deadlockDump is #230 as a stuck call's dump shows it (debug=2), cut to
// its three goroutines: the hasher, the remove, an open behind it.
const deadlockDump = `RemoveTorrent (request label s1.vfs443) was still running 5s past its bound. Every goroutine:

goroutine 15868 [sync.Mutex.Lock, 1 minutes]:
internal/sync.runtime_SemacquireMutex(0xc0001d4688?, 0x48?, 0xc0001d46b8?)
sync.(*Mutex).Lock(0x102e01040?)
github.com/shapedtime/momoshtrem/internal/torrent.(*keyedMutex).Lock(0xc00039e690, {0xc000ab7b60, 0x28})
github.com/shapedtime/momoshtrem/internal/torrent.(*service).add(0xc00039e640, {0xc000ab7b60, 0x28}, {0xc000052180, 0x6a})
created by net/http.(*Server).Serve in goroutine 107

goroutine 15922 [sync.RWMutex.RLock, 1 minutes]:
sync.runtime_SemacquireRWMutexR(0xc000bb4ec8?, 0x8?, 0xc000bb4ed8?)
sync.(*RWMutex).RLock(0xc000235528)
github.com/anacrolix/torrent/storage.piecePerResourcePiece.NewReader({{0xc0003161c0, 0x17}})
github.com/anacrolix/torrent/storage.piecePerResourcePiece.ReadAt({{0xc0003161c0, 0x17}})
io.(*SectionReader).Read(0xc00055fd70, {0xc00093a000, 0x8000, 0x8000})
github.com/anacrolix/torrent/storage.piecePerResourcePiece.WriteTo({{0xc0003161c0, 0x17}})
github.com/anacrolix/torrent.(*Torrent).pieceHasher(0xc0007d5208, 0x4)
created by github.com/anacrolix/torrent.(*Torrent).startSinglePieceHasher in goroutine 13561

goroutine 16196 [sync.RWMutex.Lock, 1 minutes]:
sync.runtime_SemacquireRWMutex(0x0?, 0x0?, 0xc000abd878?)
sync.(*RWMutex).Lock(0xc000235528)
github.com/anacrolix/torrent/storage.piecePerResourcePiece.MarkNotComplete({{0xc0003161c0, 0x17}})
github.com/shapedtime/momoshtrem/internal/torrent.(*service).deletePieces(0xc00039e640, {0xc000048300, 0x28})
github.com/shapedtime/momoshtrem/internal/torrent.(*service).RemoveTorrent(0xc00039e640, {0xc000048300, 0x28}, 0x1)
created by github.com/shapedtime/momoshtrem/internal/streamlab/scenario.(*stressRun).call in goroutine 1
`

func TestDeleteDeadlock(t *testing.T) {
	if !deleteDeadlock(deadlockDump) {
		t.Fatal("the cycle wasn't found")
	}
	// Two mutexes: a writer and a reader that don't block each other.
	other := strings.Replace(deadlockDump, "RLock(0xc000235528)", "RLock(0xc0002ffff0)", 1)
	if deleteDeadlock(other) {
		t.Error("a cycle across two mutexes")
	}
	// The writer isn't deletePieces' (anacrolix's own MarkComplete, say).
	notOurs := strings.Replace(deadlockDump, "internal/torrent.(*service).deletePieces", "anacrolix/torrent.(*Torrent).pieceHashed", 1)
	if deleteDeadlock(notOurs) {
		t.Error("a writer other than deletePieces matched")
	}
	// The reader isn't the hasher's double read lock.
	notHash := strings.Replace(deadlockDump, "storage.piecePerResourcePiece.WriteTo", "anacrolix/torrent.(*reader).readOnceAt", 1)
	if deleteDeadlock(notHash) {
		t.Error("a reader other than WriteTo's matched")
	}
	if deleteDeadlock("goroutine 1 [running]:\nmain.main()\n") {
		t.Error("an empty dump matched")
	}
	// A build without -race inlines RLock and prints its argument as
	// "...": that side matches any address.
	inlined := strings.Replace(deadlockDump, "RLock(0xc000235528)", "RLock(...)", 1)
	if !deleteDeadlock(inlined) {
		t.Error("an inlined RLock(...) didn't match")
	}
	// MarkNotComplete further down a goroutine blocked elsewhere isn't the
	// cycle: only where it blocks counts.
	elsewhere := strings.Replace(deadlockDump, "sync.(*RWMutex).Lock(0xc000235528)\n",
		"sync.(*RWMutex).Lock(0xc000235528)\ngithub.com/shapedtime/momoshtrem/internal/torrent/piececache.(*Cache).Remove(0x1)\n", 1)
	if deleteDeadlock(elsewhere) {
		t.Error("a writer blocked in the piece cache matched")
	}
}

// choices draws what a stress script would: per round a release moment
// from the script's strand, then per worker (visited in order) an offset
// and a size from the worker's strand of that round; then the pollers'
// picks. Keyed by who drew what.
func choices(base uint64, order []int, complete func(off int64) bool) map[string]int64 {
	out := map[string]int64{}
	script := strand(base, strandScript)
	for round := 1; round <= 3; round++ {
		out[fmt.Sprintf("release %d", round)] = intn(script, 12e9)
		for _, w := range order {
			r := strand(base, strandWorker, uint64(round), uint64(w))
			out[fmt.Sprintf("offset %d/%d", round, w)] = pickOffset(r, 256<<20, complete)
			out[fmt.Sprintf("size %d/%d", round, w)] = intn(r, 16)
		}
	}
	poll := strand(base, strandPoll)
	for i := range 5 {
		out[fmt.Sprintf("poll %d", i)] = intn(poll, 12)
	}
	return out
}

// --set seed=N replays a stress script's choices: the same seed gives the
// same offsets, sizes, release moments and polls, whichever order the
// workers ran in. What had downloaded moves an offset to another
// candidate, but no strand's later choices.
func TestChoicesReplay(t *testing.T) {
	none := func(int64) bool { return false }
	a := choices(42, []int{0, 1, 2}, none)
	b := choices(42, []int{2, 0, 1}, none)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("the same seed, workers in another order:\n%v\n%v", a, b)
	}
	if c := choices(43, []int{0, 1, 2}, none); fmt.Sprint(c) == fmt.Sprint(a) {
		t.Error("another seed made the same choices")
	}
	// Half the file complete: offsets move to incomplete pieces, sizes
	// and the other strands stay.
	half := func(off int64) bool { return off < 128<<20 }
	d := choices(42, []int{0, 1, 2}, half)
	for k, v := range a {
		switch {
		case strings.HasPrefix(k, "offset"):
			if d[k] < 128<<20 {
				t.Errorf("%s = %d, a complete piece", k, d[k])
			}
		case d[k] != v:
			t.Errorf("%s = %d with half the file complete, %d without", k, d[k], v)
		}
	}
	if p := pickOffset(strand(1, 9), 256<<20, none); p%(64<<10) != 0 || p < 0 || p >= 256<<20 {
		t.Errorf("pickOffset = %d", p)
	}
}

func TestSplitCall(t *testing.T) {
	for line, want := range map[string]call{
		"sync.(*RWMutex).Lock(0xc000235528)":          {"sync.(*RWMutex).Lock", "0xc000235528"},
		"sync.(*RWMutex).RLock(...)":                  {"sync.(*RWMutex).RLock", "..."},
		"pkg.piece.NewReader({{0xc0003161c0, 0x17}})": {"pkg.piece.NewReader", "{{0xc0003161c0, 0x17}}"},
		"pkg.(*T).f.func1()":                          {"pkg.(*T).f.func1", ""},
		"created by pkg.f in goroutine 3":             {"created by pkg.f in goroutine 3", ""},
	} {
		if got := splitCall(line); got != want {
			t.Errorf("splitCall(%q) = %+v, want %+v", line, got, want)
		}
	}
}

func TestStuckKnown(t *testing.T) {
	for _, tt := range []struct {
		stuck []stuckCall
		want  string
	}{
		{nil, ""},
		{[]stuckCall{{"RemoveTorrent", true}, {"Open", true}}, "#230"},
		{[]stuckCall{{"RemoveTorrent", true}, {"Open", false}}, ""},     // the open got stuck before the cycle
		{[]stuckCall{{"RemoveTorrent", true}, {"ReadAt(…)", true}}, ""}, // a read stuck too: something else
		{[]stuckCall{{"Close", true}}, ""},                              // a close isn't behind the hash lock
	} {
		if got := stuckKnown(tt.stuck); got != tt.want {
			t.Errorf("stuckKnown(%v) = %q, want %q", tt.stuck, got, tt.want)
		}
	}
}

func TestQuietKnown(t *testing.T) {
	// leak is a debug=1 record of one goroutine with these frames,
	// innermost first.
	leak := func(frames ...string) checks.LabelledGoroutines {
		var b strings.Builder
		b.WriteString("1 @ 0x1\n# labels: {\"slreq\":\"s1.9\"}")
		for i, f := range frames {
			fmt.Fprintf(&b, "\n#\t0x%x\t%s+0x1\t/x.go:%d", i+1, f, i+1)
		}
		return checks.LabelledGoroutines{Request: "s1.9", Count: 1, Stack: b.String()}
	}
	remove := leak("sync.runtime_SemacquireRWMutex", "sync.(*RWMutex).Lock", markNotComplete, deletePieces,
		"github.com/shapedtime/momoshtrem/internal/torrent.(*service).RemoveTorrent")
	open := leak("internal/sync.runtime_SemacquireMutex", "sync.(*Mutex).Lock", hashLock,
		"github.com/shapedtime/momoshtrem/internal/torrent.(*service).add", sharedAdd)
	waiter := leak("sync.runtime_SemacquireWaitGroup", "sync.(*WaitGroup).Wait", sharedAdd, addTorrent)
	// Another package's singleflight wait isn't an add queued behind the
	// remove.
	elsewhere := leak("sync.runtime_SemacquireWaitGroup", "sync.(*WaitGroup).Wait", sharedAdd,
		"github.com/shapedtime/momoshtrem/internal/search.(*Service).run")
	orphan := leak("runtime.gopark", "github.com/anacrolix/torrent.(*reader).waitAvailable",
		"github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1")
	other := leak("github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).detectFormat")
	// An add waiting for its metadata holds the hash lock, with
	// singleflight on its stack, but it isn't queued behind the remove.
	adding := leak("runtime.gopark", "runtime.selectgo", "github.com/shapedtime/momoshtrem/internal/torrent.(*service).add",
		hashLock, sharedAdd)
	q := func(gs ...checks.LabelledGoroutines) checks.QuiescenceResult {
		return checks.QuiescenceResult{Requests: 2, InFlight: 2, Leaked: gs}
	}
	for _, tt := range []struct {
		name     string
		q        checks.QuiescenceResult
		timeouts float64
		cycle    bool
		want     string
	}{
		{"the remove and what waits behind it", q(remove, open, waiter), 0, true, "#230"},
		{"without the cycle seen", q(remove, open, waiter), 0, false, ""},
		{"with #225's orphan beside", q(remove, open, orphan), 1, true, "#225+#230"},
		{"an orphan without a timed-out read", q(remove, orphan), 0, true, ""},
		{"something else left", q(remove, other), 0, true, ""},
		{"an add blocked elsewhere, the hash lock on its stack", q(remove, adding), 0, true, ""},
		{"another package's singleflight wait", q(remove, elsewhere), 0, true, ""},
		{"nothing of the deadlock", q(orphan), 1, true, ""},
		{"a reader left open", checks.QuiescenceResult{Readers: 1, Leaked: []checks.LabelledGoroutines{remove}}, 0, true, ""},
	} {
		if got := quietKnown(tt.q, tt.timeouts, tt.cycle); got != tt.want {
			t.Errorf("%s: quietKnown = %q, want %q", tt.name, got, tt.want)
		}
	}
}
