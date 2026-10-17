//go:build unix

package runner

import (
	"os"
	"syscall"
)

// quit asks the Go process p to exit with a dump of every goroutine
// (SIGQUIT, with GOTRACEBACK=all).
func quit(p *os.Process) { p.Signal(syscall.SIGQUIT) }
