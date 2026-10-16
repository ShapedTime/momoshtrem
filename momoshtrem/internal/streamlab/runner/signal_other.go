//go:build !unix

package runner

import "os"

// quit kills p: without SIGQUIT there's no goroutine dump.
func quit(p *os.Process) { p.Kill() }
