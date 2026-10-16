package piececache

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// bruteNames is upstream's Readdirnames over a set of keys: every key
// under dir + "/", relative to it.
func bruteNames(keys map[key]bool, dir string) []string {
	var names []string
	for k := range keys {
		if rest, ok := strings.CutPrefix(string(k), dir+"/"); ok {
			names = append(names, rest)
		}
	}
	slices.Sort(names)
	return names
}

func TestDirIndexMatchesBruteForce(t *testing.T) {
	// "x/a" and "x/a/b" can both be items when files change outside the
	// cache; the index must hold both.
	pool := []key{"a", "b", "x/a", "x/a/b", "x/a/c", "x/b", "y/z/w", "y/z", "completed/00", "incompleted/00/0", "incompleted/00/16"}
	dirs := []string{"x", "x/a", "y", "y/z", "completed", "incompleted", "incompleted/00", "nope"}
	r := rand.New(rand.NewPCG(1, 2))
	d := make(dirIndex)
	keys := make(map[key]bool)
	for step := range 2000 {
		k := pool[r.IntN(len(pool))]
		if keys[k] {
			d.remove(k)
			delete(keys, k)
		} else {
			d.add(k)
			keys[k] = true
		}
		for _, dir := range dirs {
			got := d.names(key(dir))
			slices.Sort(got)
			if want := bruteNames(keys, dir); !slices.Equal(got, want) {
				t.Fatalf("step %d: names(%q) = %v, want %v (keys %v)", step, dir, got, want, keys)
			}
		}
	}
	for k := range keys {
		d.remove(k)
	}
	if len(d) != 0 {
		t.Errorf("index not empty after removing every key: %v", d)
	}
}

func TestDirIndexFiles(t *testing.T) {
	d := make(dirIndex)
	for _, k := range []key{"incompleted/00/0", "incompleted/00/16", "incompleted/00/sub/1", "incompleted/01/0"} {
		d.add(k)
	}
	got := slices.Sorted(d.files("incompleted/00"))
	if want := []string{"0", "16"}; !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}
