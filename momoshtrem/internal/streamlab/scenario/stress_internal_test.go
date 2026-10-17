package scenario

import (
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
	leak := func(stack string) checks.LabelledGoroutines {
		return checks.LabelledGoroutines{Request: "s1.9", Count: 1, Stack: stack}
	}
	remove := leak("#\t0x1\tgithub.com/shapedtime/momoshtrem/internal/torrent.(*service).deletePieces+0x1\t/x.go:1")
	open := leak("#\t0x1\tgithub.com/shapedtime/momoshtrem/internal/torrent.(*keyedMutex).Lock+0x1\t/x.go:1")
	waiter := leak("#\t0x1\tgolang.org/x/sync/singleflight.(*Group).Do+0x1\t/x.go:1")
	orphan := leak("#\t0x1\tgithub.com/anacrolix/torrent.(*reader).waitAvailable+0x1\t/r.go:1\n" +
		"#\t0x2\tgithub.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1+0x1\t/t.go:1")
	other := leak("#\t0x1\tgithub.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).detectFormat+0x1\t/d.go:1")
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
		{"nothing of the deadlock", q(orphan), 1, true, ""},
		{"a reader left open", checks.QuiescenceResult{Readers: 1, Leaked: []checks.LabelledGoroutines{remove}}, 0, true, ""},
	} {
		if got := quietKnown(tt.q, tt.timeouts, tt.cycle); got != tt.want {
			t.Errorf("%s: quietKnown = %q, want %q", tt.name, got, tt.want)
		}
	}
}
