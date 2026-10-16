package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func releaseOptions() Options {
	return Options{Now: func() time.Time {
		return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	}}
}

// A second open in the same process waits for the file lock; once the
// first handle has closed, Done is closed and the reopen succeeds.
func TestStoreDoneClosesWhenCloseReturnsAndFreesTheLock(t *testing.T) {
	previous := openTimeout
	openTimeout = 50 * time.Millisecond
	defer func() { openTimeout = previous }()
	path := filepath.Join(t.TempDir(), "state.db")
	first, err := Open(path, releaseOptions())
	require.NoError(t, err)

	select {
	case <-first.Done():
		t.Fatal("Done closed before Close")
	default:
	}
	_, err = Open(path, releaseOptions())
	require.True(t, IsLockTimeout(err), "held file: %v", err)

	require.NoError(t, first.Close())
	select {
	case <-first.Done():
	default:
		t.Fatal("Done not closed after Close returned")
	}
	require.NoError(t, first.Close(), "Close is idempotent")
	again, err := Open(path, releaseOptions())
	require.NoError(t, err)
	require.NoError(t, again.Close())
}

func TestIsLockTimeoutIgnoresOtherErrors(t *testing.T) {
	require.False(t, IsLockTimeout(nil))
	require.False(t, IsLockTimeout(ErrFenced))
}
