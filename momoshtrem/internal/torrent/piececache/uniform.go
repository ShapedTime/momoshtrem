package piececache

import (
	"io"
	"os"

	"github.com/anacrolix/missinggo/v2/resource"
)

type uniformResourceProvider struct {
	*Cache
}

var _ resource.Provider = &uniformResourceProvider{}

func (me *uniformResourceProvider) NewInstance(loc string) (resource.Instance, error) {
	return &uniformResource{me.Cache, loc}, nil
}

type uniformResource struct {
	Cache    *Cache
	Location string
}

func (me *uniformResource) Get() (io.ReadCloser, error) {
	return me.Cache.OpenFile(me.Location, os.O_RDONLY)
}

func (me *uniformResource) Put(r io.Reader) (err error) {
	f, err := me.Cache.OpenFile(me.Location, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return
}

func (me *uniformResource) ReadAt(b []byte, off int64) (n int, err error) {
	f, err := me.Cache.OpenFile(me.Location, os.O_RDONLY)
	if err != nil {
		return
	}
	defer f.Close()
	return f.ReadAt(b, off)
}

func (me *uniformResource) WriteAt(b []byte, off int64) (n int, err error) {
	f, err := me.Cache.OpenFile(me.Location, os.O_CREATE|os.O_WRONLY)
	if err != nil {
		return
	}
	defer f.Close()
	return f.WriteAt(b, off)
}

func (me *uniformResource) Stat() (fi os.FileInfo, err error) {
	return me.Cache.Stat(me.Location)
}

func (me *uniformResource) Delete() error {
	return me.Cache.Remove(me.Location)
}

// piececache: Readdirnames reads the directory index instead of walking
// every item. Upstream matches each item's path against Location + "/", so
// only a clean, non-empty Location matches anything: item paths are
// sanitized and never start with "/".
func (me *uniformResource) Readdirnames() (names []string, err error) {
	if me.Location == "" || string(sanitizePath(me.Location)) != me.Location {
		return
	}
	me.Cache.mu.Lock()
	defer me.Cache.mu.Unlock()
	return me.Cache.dirs.names(key(me.Location)), nil
}
