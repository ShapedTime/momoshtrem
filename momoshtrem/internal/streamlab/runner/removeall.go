package runner

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tempPrefix names a repetition's temp folder (runRep).
const tempPrefix = "streamlab-rep-"

// removeOwnTemp removes this child's TMPDIR when it's the folder the
// runner made for it: a child whose parent died cleans up after itself.
func removeOwnTemp() {
	if dir := os.Getenv("TMPDIR"); dir != "" && strings.HasPrefix(filepath.Base(filepath.Clean(dir)), tempPrefix) {
		os.RemoveAll(dir)
	}
}

// removeAll removes dir, retrying a while: a child that was killed may
// still have had files open a moment ago.
func removeAll(dir string) {
	for range 20 {
		if os.RemoveAll(dir) == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
