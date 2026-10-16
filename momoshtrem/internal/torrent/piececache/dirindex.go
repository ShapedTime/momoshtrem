package piececache

import "strings"

// piececache: the directory index, so that Readdirnames costs the
// directory's own entries. Upstream walks every item in the cache, under
// the lock, on each call, and anacrolix calls it on every 32 KiB read of a
// piece being hashed: at 150 GiB that's ~38k items per read.

// dirIndex holds the entries under each directory of the cache's items. An
// item "a/b/c" puts the file "c" under "a/b", the directory "b" under "a"
// and the directory "a" under "" (the root).
type dirIndex map[key]map[string]dirEntry

// dirEntry says what a name under a directory is. It can be both: the item
// map can briefly hold "a" and "a/b" when files change outside the cache.
type dirEntry struct {
	file bool
	dir  bool
}

// splitKey splits k into its directory ("" at the root) and its name.
func splitKey(k key) (dir key, name string) {
	i := strings.LastIndexByte(string(k), '/')
	if i < 0 {
		return "", string(k)
	}
	return k[:i], string(k[i+1:])
}

// add records the item k, and its directories up to the root.
func (d dirIndex) add(k key) {
	dir, name := splitKey(k)
	d.set(dir, name, func(e *dirEntry) { e.file = true })
	for dir != "" {
		var parent key
		parent, name = splitKey(dir)
		if e, ok := d[parent][name]; ok && e.dir {
			return
		}
		d.set(parent, name, func(e *dirEntry) { e.dir = true })
		dir = parent
	}
}

func (d dirIndex) set(dir key, name string, f func(*dirEntry)) {
	entries, ok := d[dir]
	if !ok {
		entries = make(map[string]dirEntry)
		d[dir] = entries
	}
	e := entries[name]
	f(&e)
	entries[name] = e
}

// remove forgets the item k, and every directory it leaves empty.
func (d dirIndex) remove(k key) {
	dir, name := splitKey(k)
	unset := func(e *dirEntry) { e.file = false }
	for {
		entries := d[dir]
		e, ok := entries[name]
		if !ok {
			return
		}
		unset(&e)
		if e.file || e.dir {
			entries[name] = e
			return
		}
		delete(entries, name)
		if len(entries) > 0 || dir == "" {
			if len(entries) == 0 {
				delete(d, dir)
			}
			return
		}
		delete(d, dir)
		dir, name = splitKey(dir)
		unset = func(e *dirEntry) { e.dir = false }
	}
}

// names returns the paths of the files under dir, relative to it and at
// any depth, as upstream's Readdirnames does by matching every item's path
// against dir + "/". The order is unspecified, as upstream's map order is.
func (d dirIndex) names(dir key) (names []string) {
	var walk func(dir key, prefix string)
	walk = func(dir key, prefix string) {
		for name, e := range d[dir] {
			if e.file {
				names = append(names, prefix+name)
			}
			if e.dir {
				walk(dir+"/"+key(name), prefix+name+"/")
			}
		}
	}
	walk(dir, "")
	return
}

// files yields the names directly under dir that are files.
func (d dirIndex) files(dir key) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for name, e := range d[dir] {
			if e.file && !yield(name) {
				return
			}
		}
	}
}
