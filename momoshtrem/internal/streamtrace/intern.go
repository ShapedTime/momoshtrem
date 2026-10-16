package streamtrace

import (
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// ID is an interned string. 0 is none: the empty string, a string interned
// while the recorder was off, or one the full table refused.
type ID uint32

// The string table only grows, so it's bounded: by count and by bytes,
// with each string cut to maxStringLen. Intern only bounded vocabularies
// (paths, User-Agents, info hashes, hosts), never per-connection values
// such as host:port.
const (
	maxStrings     = 1<<16 - 1
	maxStringBytes = 4 << 20
	maxStringLen   = 512
)

type strtab struct {
	mu      sync.RWMutex
	ids     map[string]ID
	strs    []string // strs[id-1]; append-only
	bytes   int
	refused atomic.Uint64
}

// Intern returns s's ID, the same for every call with the same string. It
// may take a lock and allocates the first time it sees a string, so hooks
// call it when a request or file opens, not per read. It returns 0 while
// the recorder is off, so production at LevelOff keeps no strings.
func (r *Recorder) Intern(s string) ID {
	if r.level.Load() == int32(LevelOff) {
		return 0
	}
	return r.strs.intern(s)
}

func (t *strtab) intern(s string) ID {
	if s == "" {
		return 0
	}
	if len(s) > maxStringLen {
		cut := maxStringLen
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut]
	}
	t.mu.RLock()
	id, ok := t.ids[s]
	t.mu.RUnlock()
	if ok {
		return id
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if id, ok := t.ids[s]; ok {
		return id
	}
	if len(t.strs) >= maxStrings || t.bytes+len(s) > maxStringBytes {
		t.refused.Add(1)
		return 0
	}
	s = strings.Clone(s) // don't pin the caller's buffer
	if t.ids == nil {
		t.ids = make(map[string]ID)
	}
	t.strs = append(t.strs, s)
	id = ID(len(t.strs))
	t.ids[s] = id
	t.bytes += len(s)
	return id
}

// snapshot returns the strings interned so far, indexed by ID-1. The slice
// is safe to read without the lock: entries never change once appended.
func (t *strtab) snapshot() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.strs
}

func (t *strtab) stats() (n int, refused uint64) {
	t.mu.RLock()
	n = len(t.strs)
	t.mu.RUnlock()
	return n, t.refused.Load()
}
