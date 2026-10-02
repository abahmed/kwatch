//go:build unix

package storage

import (
	"path/filepath"
	"syscall"
)

// osFreeSpace reports the bytes available to an unprivileged writer on the
// volume that holds path.
func osFreeSpace(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(filepath.Dir(path), &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
