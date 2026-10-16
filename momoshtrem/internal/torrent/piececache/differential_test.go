package piececache

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/missinggo/v2/filecache"
	"github.com/anacrolix/missinggo/v2/resource"
)

// The differential test: random operations on upstream's filecache and on
// piececache, in two temp folders, with no pins and a reserve of 0. After
// every step both hold the same files with the same contents, the same
// items and LRU order, and the same Info, and every operation answered the
// same (errors, bytes read, Readdirnames).
//
// The LRU orders are compared by rank, since the two caches stamp their
// own times. Every stamp comes from time.Now() and is preceded by a system
// call, so there are no ties. Rename is the one operation that stamps an
// item with its file's times (upstream stats the new name), which the two
// folders don't share: the test renames only when the cache is within its
// capacity, so that rename's own update trims nothing, and then reopens the
// new name, which stamps it with time.Now() in both.

// diffKeys is the key pool. No key is a directory of another, so no
// operation fails halfway and leaves upstream's asynchronous prune behind.
var diffKeys = []string{
	"a", "b", "c", "d/x", "d/y", "e/f/g",
	"completed/0a", "completed/0b",
	"incompleted/0a/0", "incompleted/0a/16", "incompleted/0b/0",
}

// diffDirs are the Readdirnames locations, some clean, some not.
var diffDirs = []string{
	"d", "e", "e/f", "completed", "incompleted", "incompleted/0a", "incompleted/0b",
	"", "/d", "d/", "./d", "nope", "a",
}

var diffCapacities = []int64{-1, 0, 16, 40, 64, 100, 150, 250, 400}

// diffFile is what both caches' files do.
type diffFile interface {
	io.Reader
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Closer
}

// diffCache is one side: the same operations over either cache.
type diffCache struct {
	name        string
	root        string
	open        func(path string, flag int) (diffFile, error)
	remove      func(path string) error
	rename      func(from, to string) error
	setCapacity func(int64)
	trim        func()
	info        func() (capacity, filled int64, items int, pinned int64)
	walk        func(func(path string, accessed time.Time, size int64))
	provider    resource.Provider
	// held is a file left open across steps, or nil.
	held diffFile
}

// instance is the resource at loc. Both providers never fail.
func (c *diffCache) instance(loc string) resource.Instance {
	i, err := c.provider.NewInstance(loc)
	if err != nil {
		panic(err)
	}
	return i
}

func newUpstreamSide(t *testing.T) *diffCache {
	root := t.TempDir()
	c, err := filecache.NewCache(root)
	if err != nil {
		t.Fatal(err)
	}
	return &diffCache{
		name: "upstream",
		root: root,
		open: func(p string, flag int) (diffFile, error) {
			f, err := c.OpenFile(p, flag)
			if err != nil {
				return nil, err
			}
			return f, nil
		},
		remove:      c.Remove,
		rename:      c.Rename,
		setCapacity: c.SetCapacity,
		trim:        c.TrimToCapacity,
		info: func() (int64, int64, int, int64) {
			i := c.Info()
			return i.Capacity, i.Filled, i.NumItems, 0
		},
		walk: func(f func(string, time.Time, int64)) {
			c.WalkItems(func(i filecache.ItemInfo) { f(fmt.Sprint(i.Path), i.Accessed, i.Size) })
		},
		provider: c.AsResourceProvider(),
	}
}

func newPiececacheSide(t *testing.T) *diffCache {
	root := t.TempDir()
	c, err := NewCache(root)
	if err != nil {
		t.Fatal(err)
	}
	return &diffCache{
		name: "piececache",
		root: root,
		open: func(p string, flag int) (diffFile, error) {
			f, err := c.OpenFile(p, flag)
			if err != nil {
				return nil, err
			}
			return f, nil
		},
		remove:      c.Remove,
		rename:      c.Rename,
		setCapacity: c.SetCapacity,
		trim:        c.TrimToCapacity,
		info: func() (int64, int64, int, int64) {
			i := c.Info()
			return i.Capacity, i.Filled, i.NumItems, i.Pinned
		},
		walk: func(f func(string, time.Time, int64)) {
			c.WalkItems(func(i ItemInfo) { f(string(i.Path), i.Accessed, i.Size) })
		},
		provider: c.AsResourceProvider(),
	}
}

// result is what one operation answered on one side.
type result struct {
	err  bool
	n    int
	data []byte
	// names are Readdirnames' answers, sorted.
	names []string
}

func errResult(err error) result { return result{err: err != nil} }

// diffOp is one random operation, applied to a side.
type diffOp struct {
	desc  string
	apply func(c *diffCache) result
}

func payload(r *rand.Rand, max int) []byte {
	b := make([]byte, 1+r.IntN(max))
	for i := range b {
		b[i] = byte('a' + r.IntN(26))
	}
	return b
}

// randomOp picks an operation. upstream is consulted only for the rename
// guard, which needs the fill and the capacity.
func randomOp(r *rand.Rand, upstream *diffCache) diffOp {
	k := diffKeys[r.IntN(len(diffKeys))]
	switch n := r.IntN(100); {
	case n < 22: // a chunk write, as WriteAt through the resource does
		data, off := payload(r, 48), int64(r.IntN(48))
		return diffOp{fmt.Sprintf("write %q %d@%d", k, len(data), off), func(c *diffCache) result {
			f, err := c.open(k, os.O_CREATE|os.O_WRONLY)
			if err != nil {
				return errResult(err)
			}
			defer f.Close()
			n, err := f.WriteAt(data, off)
			return result{err: err != nil, n: n}
		}}
	case n < 32: // a Put: truncate and write
		data := payload(r, 64)
		return diffOp{fmt.Sprintf("put %q %d", k, len(data)), func(c *diffCache) result {
			return errResult(c.instance(k).Put(bytes.NewReader(data)))
		}}
	case n < 47: // a read
		buf, off := make([]byte, 1+r.IntN(64)), int64(r.IntN(64))
		return diffOp{fmt.Sprintf("read %q %d@%d", k, len(buf), off), func(c *diffCache) result {
			f, err := c.open(k, os.O_RDONLY)
			if err != nil {
				return errResult(err)
			}
			defer f.Close()
			n, err := f.ReadAt(buf, off)
			return result{err: err != nil, n: n, data: slices.Clone(buf[:n])}
		}}
	case n < 52: // open, creating
		return diffOp{fmt.Sprintf("create %q", k), func(c *diffCache) result {
			f, err := c.open(k, os.O_CREATE|os.O_RDWR)
			if err != nil {
				return errResult(err)
			}
			return errResult(f.Close())
		}}
	case n < 60:
		return diffOp{fmt.Sprintf("remove %q", k), func(c *diffCache) result {
			return errResult(c.remove(k))
		}}
	case n < 64:
		to := diffKeys[r.IntN(len(diffKeys))]
		capacity, filled, _, _ := upstream.info()
		if capacity >= 0 && filled > capacity {
			return diffOp{"trim (for a rename over capacity)", func(c *diffCache) result {
				c.trim()
				return result{}
			}}
		}
		return diffOp{fmt.Sprintf("rename %q %q", k, to), func(c *diffCache) result {
			err := c.rename(k, to)
			if err == nil {
				if f, err := c.open(to, os.O_RDONLY); err == nil {
					f.Close()
				}
			}
			return errResult(err)
		}}
	case n < 72:
		capacity := diffCapacities[r.IntN(len(diffCapacities))]
		return diffOp{fmt.Sprintf("set capacity %d", capacity), func(c *diffCache) result {
			c.setCapacity(capacity)
			return result{}
		}}
	case n < 76:
		return diffOp{"trim", func(c *diffCache) result {
			c.trim()
			return result{}
		}}
	case n < 82: // Readdirnames through the resource, as anacrolix lists chunks
		dir := diffDirs[r.IntN(len(diffDirs))]
		return diffOp{fmt.Sprintf("readdirnames %q", dir), func(c *diffCache) result {
			names, err := c.instance(dir).(resource.DirInstance).Readdirnames()
			slices.Sort(names)
			return result{err: err != nil, names: names}
		}}
	case n < 86: // the resource's Get and Stat
		return diffOp{fmt.Sprintf("get %q", k), func(c *diffCache) result {
			i := c.instance(k)
			if _, err := i.Stat(); err != nil {
				return errResult(err)
			}
			rc, err := i.Get()
			if err != nil {
				return errResult(err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			return result{err: err != nil, data: data}
		}}
	case n < 89:
		return diffOp{fmt.Sprintf("delete %q", k), func(c *diffCache) result {
			return errResult(c.instance(k).Delete())
		}}
	default: // a file held open across steps: open it, use it or close it
		data, off := payload(r, 32), int64(r.IntN(32))
		action := r.IntN(4)
		return diffOp{fmt.Sprintf("held %d %q %d@%d", action, k, len(data), off), func(c *diffCache) result {
			switch {
			case c.held == nil:
				f, err := c.open(k, os.O_CREATE|os.O_RDWR)
				if err != nil {
					return errResult(err)
				}
				c.held = f
				return result{}
			case action == 0:
				err := c.held.Close()
				c.held = nil
				return errResult(err)
			case action == 1:
				buf := make([]byte, len(data))
				n, err := c.held.ReadAt(buf, off)
				return result{err: err != nil, n: n, data: buf[:n]}
			default:
				n, err := c.held.WriteAt(data, off)
				return result{err: err != nil, n: n}
			}
		}}
	}
}

// diskTree lists every file and folder under root with its contents.
func diskTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := make(map[string]string)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			tree[rel] = "<dir>"
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		tree[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// itemsOf is the items with their sizes, and their keys in LRU order:
// oldest first, ties by key, as lru orders them.
func itemsOf(c *diffCache) (sizes map[string]int64, order []string) {
	type item struct {
		path     string
		accessed time.Time
	}
	var all []item
	sizes = make(map[string]int64)
	c.walk(func(p string, accessed time.Time, size int64) {
		sizes[p] = size
		all = append(all, item{p, accessed})
	})
	slices.SortFunc(all, func(a, b item) int {
		if c := a.accessed.Compare(b.accessed); c != 0 {
			return c
		}
		return cmp.Compare(a.path, b.path)
	})
	for _, i := range all {
		order = append(order, i.path)
	}
	return
}

func compareSides(t *testing.T, u, p *diffCache) {
	t.Helper()
	if ut, pt := diskTree(t, u.root), diskTree(t, p.root); !mapsEqual(ut, pt) {
		t.Fatalf("files differ:\nupstream   %v\npiececache %v", ut, pt)
	}
	uc, uf, un, _ := u.info()
	pc, pf, pn, pp := p.info()
	if uc != pc || uf != pf || un != pn || pp != 0 {
		t.Fatalf("Info differs: upstream {%d %d %d}, piececache {%d %d %d pinned %d}", uc, uf, un, pc, pf, pn, pp)
	}
	us, uo := itemsOf(u)
	ps, po := itemsOf(p)
	if !mapsEqual(us, ps) {
		t.Fatalf("items differ:\nupstream   %v\npiececache %v", us, ps)
	}
	if !slices.Equal(uo, po) {
		t.Fatalf("LRU order differs:\nupstream   %v\npiececache %v", uo, po)
	}
}

func mapsEqual[V comparable](a, b map[string]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || v != w {
			return false
		}
	}
	return true
}

func TestDifferentialAgainstUpstream(t *testing.T) {
	seeds, steps := 24, 400
	if testing.Short() {
		seeds, steps = 6, 200
	}
	for seed := range seeds {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			r := rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15))
			u, p := newUpstreamSide(t), newPiececacheSide(t)
			defer func() {
				for _, c := range []*diffCache{u, p} {
					if c.held != nil {
						c.held.Close()
					}
				}
			}()
			var log []string
			for step := range steps {
				op := randomOp(r, u)
				log = append(log, op.desc)
				ur, pr := op.apply(u), op.apply(p)
				if ur.err != pr.err || ur.n != pr.n || !bytes.Equal(ur.data, pr.data) || !slices.Equal(ur.names, pr.names) {
					t.Fatalf("step %d, %s: upstream answered %+v, piececache %+v\nsteps:\n%s",
						step, op.desc, ur, pr, strings.Join(log, "\n"))
				}
				if t.Failed() {
					return
				}
				func() {
					defer func() {
						if t.Failed() {
							t.Logf("at step %d, %s; steps:\n%s", step, op.desc, strings.Join(log, "\n"))
						}
					}()
					compareSides(t, u, p)
				}()
			}
		})
	}
}
