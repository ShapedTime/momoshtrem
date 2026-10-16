package main

import (
	"os"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// The Governor drives the real piece cache through the adapter: its cap,
// its fill with the pinned part, and its trims.
func TestStorageCache_Piececache(t *testing.T) {
	fc, err := piececache.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cache := storageCache(fc)
	// Three completed pieces, oldest first.
	for _, hex := range []string{"aa", "bb", "cc"} {
		f, err := fc.OpenFile("completed/"+hex, os.O_CREATE|os.O_WRONLY)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(make([]byte, 1000)); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	fc.Pin("keep", []string{"aa"})
	cache.SetCapacity(2500)
	if info := cache.Info(); info.Capacity != 2500 || info.Filled != 3000 || info.Pinned != 1000 {
		t.Fatalf("Info = %+v: SetCapacity mustn't trim, and aa is pinned", info)
	}
	// The trim takes the oldest unpinned piece, bb, not the pinned aa.
	cache.TrimToCapacity()
	if info := cache.Info(); info.Filled != 2000 || info.Pinned != 1000 {
		t.Errorf("after the trim, Info = %+v", info)
	}
	if have := fc.HavePieces([]piececache.PieceRef{{Hex: "aa", Length: 1000}, {Hex: "bb", Length: 1000}}); !have[0] || have[1] {
		t.Errorf("after the trim, have aa %v, bb %v; want aa only", have[0], have[1])
	}
}
