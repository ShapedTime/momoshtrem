//go:build unix

package ab

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive, non-blocking flock on path (created if it
// isn't there): the file and true when it's ours, false when another
// process holds it. The kernel releases it when the holder exits, however
// it exits.
func tryLock(path string) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return f, true, nil
}

// unlock drops the lock; the file stays (see claim).
func unlock(f *os.File) { f.Close() }
