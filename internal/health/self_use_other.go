//go:build !unix

package health

// diskUse is unknown off unix.
func diskUse(string) (used, total int64, ok bool) { return 0, 0, false }
