//go:build unix

package health

import "syscall"

// diskUse is the space used and the size of the volume holding dir.
func diskUse(dir string) (used, total int64, ok bool) {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil || st.Blocks == 0 {
		return 0, 0, false
	}
	size := int64(st.Bsize)
	return int64(st.Blocks-st.Bfree) * size, int64(st.Blocks) * size, true
}
