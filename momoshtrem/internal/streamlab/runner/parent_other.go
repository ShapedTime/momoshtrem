//go:build !unix

package runner

import (
	"io"
	"os"
)

// watchParent waits for EOF on stdin, the pipe the parent holds, then
// exits: without SIGQUIT there's no goroutine dump.
func watchParent() {
	io.Copy(io.Discard, os.Stdin)
	os.Exit(ChildToolError)
}
