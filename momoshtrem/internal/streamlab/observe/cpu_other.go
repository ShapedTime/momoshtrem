//go:build !unix

package observe

import "time"

// processCPU isn't known here: the overhead is then given against the
// wall time on one core.
func processCPU() (time.Duration, bool) { return 0, false }
