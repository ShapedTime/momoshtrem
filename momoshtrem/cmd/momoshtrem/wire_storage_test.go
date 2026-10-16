package main

import (
	"os"
	"testing"

	"github.com/anacrolix/missinggo/v2/filecache"
)

// The Governor drives the real filecache through the adapter: its cap, its
// fill and its trims.
func TestStorageCache_Filecache(t *testing.T) {
	fc, err := filecache.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cache := storageCache(fc)
	for _, name := range []string{"a", "b", "c"} {
		f, err := fc.OpenFile(name, os.O_CREATE|os.O_WRONLY)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(make([]byte, 1000)); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	cache.SetCapacity(2500)
	if info := cache.Info(); info.Capacity != 2500 || info.Filled != 3000 || info.Pinned != 0 {
		t.Fatalf("Info = %+v: SetCapacity mustn't trim", info)
	}
	cache.TrimToCapacity()
	if info := cache.Info(); info.Filled != 2000 {
		t.Errorf("after the trim, Info = %+v", info)
	}
}
