//go:build unix

package activity

import "syscall"

// statfs returns the size and the space available to us of the file
// system holding path.
func statfs(path string) (total, free int64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := int64(st.Bsize) // uint32 on darwin, int64 on linux
	return int64(st.Blocks) * bsize, int64(st.Bavail) * bsize, nil
}
