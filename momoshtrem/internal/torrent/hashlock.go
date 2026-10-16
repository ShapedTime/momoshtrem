package torrent

import "sync"

// keyedMutex is a mutex per info hash. Entries are dropped once nobody holds
// or waits for them, so it doesn't grow with every torrent ever seen.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	sync.Mutex
	refs int
}

// Lock locks key and returns its unlock.
func (k *keyedMutex) Lock(key string) (unlock func()) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedLock{}
	}
	l := k.locks[key]
	if l == nil {
		l = &keyedLock{}
		k.locks[key] = l
	}
	l.refs++
	k.mu.Unlock()

	l.Lock()
	return func() {
		l.Unlock()
		k.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}
