package piececache

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Upstream's cache_test.go, ported from testify to the standard library.
// The assertions and their order are upstream's; CacheInfo has Pinned now,
// which stays 0 here.

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func wantInfo(t *testing.T, c *Cache, want CacheInfo) {
	t.Helper()
	if got := c.Info(); got != want {
		t.Fatalf("Info() = %+v, want %+v", got, want)
	}
}

func TestCache(t *testing.T) {
	td := t.TempDir()

	c, err := NewCache(filepath.Join(td, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	wantInfo(t, c, CacheInfo{Filled: 0, Capacity: -1, NumItems: 0})

	c.WalkItems(func(i ItemInfo) {})

	if _, err = c.OpenFile("/", os.O_CREATE); err == nil {
		t.Error("OpenFile(\"/\") succeeded")
	}
	if _, err = c.OpenFile("", os.O_CREATE); err == nil {
		t.Error("OpenFile(\"\") succeeded")
	}

	c.WalkItems(func(i ItemInfo) {})

	wantInfo(t, c, CacheInfo{Filled: 0, Capacity: -1, NumItems: 0})

	for _, p := range []string{"notexist", "/notexist", "/dir/notexist"} {
		if _, err = c.OpenFile(p, 0); !os.IsNotExist(err) {
			t.Errorf("OpenFile(%q) = %v, want not exist", p, err)
		}
	}

	f, err := c.OpenFile("dir/blah", os.O_CREATE)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	wantInfo(t, c, CacheInfo{Filled: 0, Capacity: -1, NumItems: 1})

	c.WalkItems(func(i ItemInfo) {})

	if !exists(filepath.Join(td, filepath.FromSlash("cache/dir/blah"))) {
		t.Error("cache/dir/blah missing")
	}
	if !exists(filepath.Join(td, filepath.FromSlash("cache/dir/"))) {
		t.Error("cache/dir missing")
	}
	if n := c.Info().NumItems; n != 1 {
		t.Errorf("NumItems = %d, want 1", n)
	}

	if _, err = f.ReadAt(nil, 0); err == io.EOF {
		t.Error("ReadAt(nil, 0) = io.EOF")
	}
	f.Close()

	if err := c.Remove("dir/blah"); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(td, filepath.FromSlash("cache/dir/blah"))) {
		t.Error("cache/dir/blah still there")
	}
	if exists(filepath.Join(td, filepath.FromSlash("cache/dir/"))) {
		t.Error("cache/dir still there")
	}

	a, err := c.OpenFile("/a", os.O_CREATE|os.O_WRONLY)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := c.OpenFile("b", os.O_CREATE|os.O_WRONLY)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	c.mu.Lock()
	if c.pathInfo("a").Accessed.After(c.pathInfo("b").Accessed) {
		t.Error("a accessed after b")
	}
	c.mu.Unlock()
	n, err := a.WriteAt([]byte("hello"), 0)
	if err != nil || n != 5 {
		t.Fatalf("WriteAt = %d, %v", n, err)
	}
	wantInfo(t, c, CacheInfo{Filled: 5, Capacity: -1, NumItems: 2})
	if c.pathInfo("b").Accessed.After(c.pathInfo("a").Accessed) {
		t.Error("b accessed after a")
	}

	// Reopen a, to check that the info values remain correct.
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = c.OpenFile("a", 0)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo(t, c, CacheInfo{Filled: 5, Capacity: -1, NumItems: 2})

	c.SetCapacity(5)
	wantInfo(t, c, CacheInfo{Filled: 5, Capacity: 5, NumItems: 2})

	if _, err = a.WriteAt([]byte(" world"), 5); err == nil {
		t.Error("WriteAt on a read-only file succeeded")
	}
	n, err = b.WriteAt([]byte("boom!"), 0)
	// "a" and "b" have been evicted.
	if err != nil || n != 5 {
		t.Fatalf("WriteAt = %d, %v", n, err)
	}
	wantInfo(t, c, CacheInfo{Filled: 5, Capacity: 5, NumItems: 1})
}

func TestSanitizePath(t *testing.T) {
	for in, want := range map[string]key{
		"////":     "",
		"/../..":   "",
		"/a//b/..": "a",
		"../a":     "a",
		"./a":      "a",
	} {
		if got := sanitizePath(in); got != want {
			t.Errorf("sanitizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func BenchmarkCacheOpenFile(b *testing.B) {
	c, err := NewCache(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		f, err := c.OpenFile("a", os.O_CREATE|os.O_RDWR)
		if err != nil {
			b.Fatal(err)
		}
		if err := f.Close(); err != nil {
			b.Error(err)
		}
	}
}

func TestFileReadWrite(t *testing.T) {
	c, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	a, err := c.OpenFile("a", os.O_CREATE|os.O_EXCL|os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	for off, c := range []byte("herp") {
		n, err := a.WriteAt([]byte{c}, int64(off))
		if err != nil || n != 1 {
			t.Fatalf("WriteAt = %d, %v", n, err)
		}
	}
	for off, c := range []byte("herp") {
		var b [1]byte
		n, err := a.ReadAt(b[:], int64(off))
		if err != nil || n != 1 {
			t.Fatalf("ReadAt = %d, %v", n, err)
		}
		if b[0] != c {
			t.Errorf("byte %d = %q, want %q", off, b[0], c)
		}
	}
}
