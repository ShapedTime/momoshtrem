//go:build unix

package runner

import (
	"io"
	"os"
	"syscall"
)

// watchParent waits for EOF on stdin, the pipe the parent holds, then
// quits this process with a goroutine dump (SIGQUIT).
func watchParent() {
	io.Copy(io.Discard, os.Stdin)
	syscall.Kill(os.Getpid(), syscall.SIGQUIT)
}
