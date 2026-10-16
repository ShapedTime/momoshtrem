package piececache

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/anacrolix/log"

	"github.com/anacrolix/missinggo/v2/pproffd"
	"github.com/anacrolix/missinggo/v2/resource"
)

// This is missinggo v2.10.0's filecache with pins, a streaming reserve,
// deferred trimming and a directory index. UPSTREAM.md lists every change;
// each one is marked "piececache:" in the code.

const (
	dirPerm  = 0755
	filePerm = 0644
)

type Cache struct {
	root     string
	mu       sync.Mutex
	capacity int64
	filled   int64
	policy   Policy
	items    map[key]itemState

	// piececache: the additions. Pinned items stay in items and filled but
	// are out of the policy, so the trim never picks them.

	// dirs holds the names under each directory, for Readdirnames.
	dirs dirIndex
	// reserve is the unpinned bytes the trim never goes below.
	reserve int64
	// pinned is the bytes of the pinned items, part of filled.
	pinned int64
	// owners holds the piece hexes each owner pins, and pinCount how many
	// owners pin each hex. A hex is pinned while its count is above 0.
	owners   map[string]map[string]struct{}
	pinCount map[string]int
	// deferring skips every trim until Ready (DeferTrim).
	deferring  bool
	readyTimer *time.Timer
}

type CacheInfo struct {
	Capacity int64
	Filled   int64
	NumItems int
	// piececache: Pinned is the bytes of pinned items, part of Filled.
	Pinned int64
}

type ItemInfo struct {
	Path     key
	Accessed time.Time
	Size     int64
}

// Calls the function for every item known to be in the cache.
func (me *Cache) WalkItems(cb func(ItemInfo)) {
	me.mu.Lock()
	defer me.mu.Unlock()
	for k, ii := range me.items {
		cb(ItemInfo{
			Path:     k,
			Accessed: ii.Accessed,
			Size:     ii.Size,
		})
	}
}

func (me *Cache) Info() (ret CacheInfo) {
	me.mu.Lock()
	defer me.mu.Unlock()
	ret.Capacity = me.capacity
	ret.Filled = me.filled
	ret.NumItems = len(me.items)
	ret.Pinned = me.pinned
	return
}

// Setting a negative capacity means unlimited.
func (me *Cache) SetCapacity(capacity int64) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.capacity = capacity
}

func NewCache(root string) (ret *Cache, err error) {
	root, err = filepath.Abs(root)
	ret = &Cache{
		root:     root,
		capacity: -1, // unlimited
		// piececache: the pin tables.
		owners:   make(map[string]map[string]struct{}),
		pinCount: make(map[string]int),
	}
	ret.mu.Lock()
	go func() {
		defer ret.mu.Unlock()
		ret.rescan()
	}()
	return
}

// An empty return path is an error.
func sanitizePath(p string) (ret key) {
	if p == "" {
		return
	}
	ret = key(path.Clean("/" + p))
	if ret[0] == '/' {
		ret = ret[1:]
	}
	return
}

// Leaf is a descendant of root.
func pruneEmptyDirs(root string, leaf string) (err error) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return
	}
	for {
		var leafInfo os.FileInfo
		leafInfo, err = os.Stat(leaf)
		if os.IsNotExist(err) {
			goto parent
		}
		if err != nil {
			return
		}
		if !leafInfo.IsDir() {
			return
		}
		if os.SameFile(rootInfo, leafInfo) {
			return
		}
		if os.Remove(leaf) != nil {
			return
		}
	parent:
		leaf = filepath.Dir(leaf)
	}
}

// Remove deletes the item even when it's pinned: a piece that fails its
// hash must go. Its hex stays pinned, so a new download of it is pinned too.
func (me *Cache) Remove(path string) error {
	me.mu.Lock()
	defer me.mu.Unlock()
	return me.remove(sanitizePath(path))
}

var (
	ErrBadPath = errors.New("bad path")
	ErrIsDir   = errors.New("is directory")
)

func (me *Cache) StatFile(path string) (os.FileInfo, error) {
	return os.Stat(me.realpath(sanitizePath(path)))
}

func isMissingDir(err error) bool {
	// I'm not sure why we get EINVAL for missing path components sometimes. It happens on MacOS. I
	// wonder if it would happen the same on other OS.
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOENT) || errors.Is(err, fs.ErrNotExist)
}

func (me *Cache) OpenFile(path string, flag int) (ret *File, err error) {
	key := sanitizePath(path)
	if key == "" {
		err = ErrIsDir
		return
	}
	filePath := me.realpath(key)
	f, err := os.OpenFile(filePath, flag, filePerm)
	// Ensure intermediate directories and try again.
	if flag&os.O_CREATE != 0 && isMissingDir(err) {
		dirPath := filepath.Dir(filePath)
		if dirPath == "" {
			panic(key)
		}
		defer func() {
			if err != nil {
				go me.pruneEmptyDirs(key)
			}
		}()
		for {
			err = os.MkdirAll(dirPath, dirPerm)
			if isMissingDir(err) {
				continue
			} else if err != nil {
				return
			}
			f, err = os.OpenFile(filePath, flag, filePerm)
			if isMissingDir(err) {
				continue
			}
			break
		}
	}
	if err != nil {
		return
	}
	ret = &File{
		path: key,
		f:    pproffd.WrapOSFile(f),
		onRead: func(n int) {
			me.mu.Lock()
			defer me.mu.Unlock()
			me.updateItem(key, func(i *itemState, ok bool) bool {
				i.Accessed = time.Now()
				return ok
			})
		},
		afterWrite: func(endOff int64) {
			me.mu.Lock()
			defer me.mu.Unlock()
			me.updateItem(key, func(i *itemState, ok bool) bool {
				i.Accessed = time.Now()
				if endOff > i.Size {
					i.Size = endOff
				}
				return ok
			})
		},
	}
	me.mu.Lock()
	defer me.mu.Unlock()
	me.updateItem(key, func(i *itemState, ok bool) bool {
		if !ok {
			*i, ok = me.statKey(key)
		}
		i.Accessed = time.Now()
		return ok
	})
	return
}

func (me *Cache) rescan() {
	me.filled = 0
	me.policy = new(lru)
	me.items = make(map[key]itemState)
	// piececache: the index is rebuilt with the items. No pins exist yet:
	// Pin waits for the rescan, which holds the lock.
	me.dirs = make(dirIndex)
	me.pinned = 0
	err := filepath.Walk(me.root, func(path string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		path, err = filepath.Rel(me.root, path)
		if err != nil {
			log.Print(err)
			return nil
		}
		key := sanitizePath(path)
		me.updateItem(key, func(i *itemState, ok bool) bool {
			if ok {
				panic("scanned duplicate items")
			}
			*i, ok = me.statKey(key)
			return ok
		})
		return nil
	})
	if err != nil {
		panic(err)
	}
}

func (me *Cache) statKey(k key) (i itemState, ok bool) {
	fi, err := os.Stat(me.realpath(k))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		panic(err)
	}
	i.FromOSFileInfo(fi)
	ok = true
	return
}

// piececache: updateItem is upstream's applied as two steps, the update and
// the trim, so that the trim can protect k (see trimToCapacity).
func (me *Cache) updateItem(k key, u func(*itemState, bool) bool) {
	me.applyUpdate(k, u)
	me.trimToCapacity(k)
}

// applyUpdate is upstream's updateItem without the trim. It also keeps the
// pinned bytes and the directory index: a pinned item counts in pinned
// instead of going to the policy.
func (me *Cache) applyUpdate(k key, u func(*itemState, bool) bool) {
	ii, ok := me.items[k]
	pinned := me.isPinned(k)
	me.filled -= ii.Size
	if pinned {
		me.pinned -= ii.Size
	}
	if u(&ii, ok) {
		me.filled += ii.Size
		if pinned {
			me.pinned += ii.Size
		} else {
			me.policy.Used(k, ii.Accessed)
		}
		me.items[k] = ii
		if !ok {
			me.dirs.add(k)
		}
	} else {
		me.policy.Forget(k)
		delete(me.items, k)
		if ok {
			me.dirs.remove(k)
		}
	}
}

func (me *Cache) realpath(path key) string {
	return filepath.Join(me.root, filepath.FromSlash(string(path)))
}

func (me *Cache) TrimToCapacity() {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.trimToCapacity("")
}

func (me *Cache) pruneEmptyDirs(path key) error {
	return pruneEmptyDirs(me.root, me.realpath(path))
}

func (me *Cache) remove(path key) error {
	err := me.removeFile(path)
	me.updateItem(path, func(*itemState, bool) bool {
		return false
	})
	return err
}

// piececache: removeFile is the file half of upstream's remove.
func (me *Cache) removeFile(path key) error {
	err := os.Remove(me.realpath(path))
	if os.IsNotExist(err) {
		err = nil
	}
	//if err != nil {
	//	return err
	//}
	pruneErr := me.pruneEmptyDirs(path)
	if err == nil {
		err = pruneErr
	}
	return err
}

// piececache: the trim. Upstream evicts the least recently used item while
// filled > capacity, through remove, whose own update trims again; this
// loop does the same evictions in the same order without the recursion, so
// that protect holds throughout. It also:
//   - evicts only unpinned items (pinned ones aren't in the policy), and only
//     while the unpinned bytes are above the reserve;
//   - stops when nothing is left to evict, where upstream's Choose panics;
//   - skips every trim while DeferTrim holds them;
//   - keeps protect, the key whose update runs this trim, when it is the
//     last unpinned item and fits the capacity on its own: only pinned bytes
//     hold the cache over its capacity then. Without pins the last item is
//     everything that's filled, so it doesn't fit and goes as upstream's does.
func (me *Cache) trimToCapacity(protect key) {
	if me.capacity < 0 || me.deferring {
		return
	}
	for me.filled > me.capacity && me.filled-me.pinned > me.reserve {
		if me.policy.NumItems() == 0 {
			return
		}
		k := me.policy.Choose().(key)
		if k == protect && me.policy.NumItems() == 1 && me.items[k].Size <= me.capacity {
			return
		}
		// We can fail to remove things on Windows. The item is forgotten
		// anyway, so the loop moves on.
		me.removeFile(k)
		me.applyUpdate(k, func(*itemState, bool) bool {
			return false
		})
	}
}

// TODO: Do I need this?
func (me *Cache) pathInfo(p string) itemState {
	return me.items[sanitizePath(p)]
}

func (me *Cache) Rename(from, to string) (err error) {
	_from := sanitizePath(from)
	_to := sanitizePath(to)
	me.mu.Lock()
	defer me.mu.Unlock()
	err = os.MkdirAll(filepath.Dir(me.realpath(_to)), dirPerm)
	if err != nil {
		return
	}
	err = os.Rename(me.realpath(_from), me.realpath(_to))
	if err != nil {
		return
	}
	// We can do a dance here to copy the state from the old item, but lets
	// just stat the new item for now.
	me.updateItem(_from, func(i *itemState, ok bool) bool {
		return false
	})
	me.updateItem(_to, func(i *itemState, ok bool) bool {
		*i, ok = me.statKey(_to)
		return ok
	})
	return
}

func (me *Cache) Stat(path string) (os.FileInfo, error) {
	return os.Stat(me.realpath(sanitizePath(path)))
}

func (me *Cache) AsResourceProvider() resource.Provider {
	return &uniformResourceProvider{me}
}
