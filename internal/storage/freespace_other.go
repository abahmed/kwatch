//go:build !unix

package storage

import "errors"

func osFreeSpace(string) (uint64, error) {
	return 0, errors.New("free space is not available on this platform")
}
