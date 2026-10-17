//go:build unix

package runner

import (
	"io"
	"os"
	"syscall"
)

// watchParent waits for EOF on stdin, the pipe the parent holds, then
// removes this repetition's temp folder (the parent can't any more) and
// quits with a goroutine dump (SIGQUIT).
func watchParent() {
	io.Copy(io.Discard, os.Stdin)
	removeOwnTemp()
	syscall.Kill(os.Getpid(), syscall.SIGQUIT)
}
