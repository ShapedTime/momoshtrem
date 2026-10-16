package piececache

import (
	"slices"
	"testing"
	"time"
)

// A restart is a new cache over the same folder: the rescan rebuilds the
// items, the LRU and the directory index from the files.

// filled downloads pieces 0 to 7 and leaves 8 with chunks only, on an
// unlimited cache.
func filled(t *testing.T) (*pieceFixture, string) {
	t.Helper()
	root := t.TempDir()
	f := newPieceFixture(t, 12, root)
	for i := range 8 {
		f.download(t, i)
	}
	f.writeChunks(t, 8)
	return f, root
}

func TestRestartRebuildsIndexes(t *testing.T) {
	f, root := filled(t)
	before := f.c.Info()

	f.open(t, root)
	if got := f.c.Info(); got != before {
		t.Fatalf("Info after restart = %+v, want %+v", got, before)
	}
	have := f.c.HavePieces(f.refs)
	for i, ok := range have {
		if want := i < 8; ok != want {
			t.Errorf("HavePieces[%d] = %v, want %v", i, ok, want)
		}
	}
	chunks := f.readdir("incompleted/" + f.refs[8].Hex)
	slices.Sort(chunks)
	if want := []string{"0", "16384"}; !slices.Equal(chunks, want) {
		t.Errorf("chunks after restart = %v, want %v", chunks, want)
	}
	if n := len(f.readdir("completed")); n != 8 {
		t.Errorf("completed holds %d names, want 8", n)
	}
	// Pins restored after the restart find the rescanned items, complete
	// and not.
	f.c.Pin("keep", f.hexes(0, 8))
	if got := f.c.Info().Pinned; got != 2*testPieceLength {
		t.Errorf("Pinned = %d, want 2 pieces", got)
	}
	// And the piece completes from its rescanned chunks.
	if err := f.piece(8).MarkComplete(); err != nil {
		t.Fatal(err)
	}
	if !f.complete(t, 8) {
		t.Error("piece 8 incomplete after MarkComplete")
	}
}

func TestRestartWithoutDeferTrimTrimsAtFirstWrite(t *testing.T) {
	f, root := filled(t)
	f.open(t, root)
	f.c.SetCapacity(3 * testPieceLength)
	if n := len(f.completeSet(t)); n != 8 {
		t.Fatalf("SetCapacity trimmed: %d pieces left", n)
	}
	// As today: the first update trims to the capacity.
	f.download(t, 9)
	if info := f.c.Info(); info.Filled > info.Capacity {
		t.Errorf("Filled %d over Capacity %d after the first write", info.Filled, info.Capacity)
	}
	if have := f.completeSet(t); len(have) > 3 || !have[9] {
		t.Errorf("complete %v, want at most 3 with 9", have)
	}
}

func TestDeferTrimHoldsUntilReady(t *testing.T) {
	f, root := filled(t)
	f.open(t, root)
	f.c.DeferTrim()
	f.c.SetCapacity(3 * testPieceLength)

	// Streaming goes on while pins are restored: nothing is trimmed, not
	// even by TrimToCapacity.
	f.download(t, 9)
	f.c.TrimToCapacity()
	if have := f.completeSet(t); len(have) != 9 {
		t.Fatalf("trimmed before Ready: complete %v", have)
	}

	f.c.Pin("keep", f.hexes(0, 1))
	f.c.Ready()
	if have := f.completeSet(t); len(have) != 9 {
		t.Fatalf("Ready trimmed by itself: complete %v", have)
	}
	f.download(t, 10)
	have := f.completeSet(t)
	if !have[0] || !have[1] || !have[10] {
		t.Errorf("after Ready, complete %v, want the pinned 0 and 1, and 10", have)
	}
	if len(have) != 3 {
		t.Errorf("after Ready, complete %v, want 3 pieces", have)
	}
}

func TestDeferTrimFallback(t *testing.T) {
	f, root := filled(t)
	f.open(t, root)
	f.c.deferTrimFor(20 * time.Millisecond)
	f.c.SetCapacity(3 * testPieceLength)
	f.download(t, 9)
	if n := len(f.completeSet(t)); n != 9 {
		t.Fatalf("trimmed before the fallback: %d pieces", n)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.c.mu.Lock()
		deferring := f.c.deferring
		f.c.mu.Unlock()
		if !deferring {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the fallback never ended DeferTrim")
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.c.TrimToCapacity()
	if info := f.c.Info(); info.Filled > info.Capacity {
		t.Errorf("Filled %d over Capacity %d after the fallback", info.Filled, info.Capacity)
	}
	// Ready after the fallback does nothing.
	f.c.Ready()
}
