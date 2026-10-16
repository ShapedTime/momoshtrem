//go:build !unix

package storagecap

import "errors"

// Statfs isn't implemented off Unix: Activity's disk is null there, and
// the storage limit can't follow the disk.
func Statfs(string) (total, free int64, err error) {
	return 0, 0, errors.New("statfs is not supported on this platform")
}
