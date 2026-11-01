//go:build !unix

package ab

import (
	"errors"
	"io/fs"
	"os"
)

// tryLock creates path exclusively: without flock, a lock a run that died
// left stays until removed by hand.
func tryLock(path string) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return f, true, nil
}

// unlock drops the lock: the file goes, so the next run can create it.
func unlock(f *os.File) {
	name := f.Name()
	f.Close()
	os.Remove(name)
}
