//go:build !unix

package activity

import "errors"

// statfs isn't implemented off Unix: the summary's disk is null there.
func statfs(string) (total, free int64, err error) {
	return 0, 0, errors.New("statfs is not supported on this platform")
}
