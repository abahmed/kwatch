package storage

import (
	"errors"

	berrors "go.etcd.io/bbolt/errors"
)

// Done is closed once Close has returned and the file lock is released,
// so a caller that reopens the file in the same process can wait for it.
func (s *Store) Done() <-chan struct{} { return s.done }

// IsLockTimeout reports whether err means another holder kept the file
// lock for the whole open timeout, which is transient when the holder is
// a previous generation of this process still closing the file.
func IsLockTimeout(err error) bool {
	return errors.Is(err, berrors.ErrTimeout)
}
