package runner

import (
	"os"
	"time"
)

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
