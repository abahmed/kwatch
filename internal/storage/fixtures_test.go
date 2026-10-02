package storage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeClock is a settable clock safe for use from worker goroutines.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = at
}

func (c *fakeClock) Advance(d time.Duration) {
	c.Set(c.Now().Add(d))
}

// claim takes the store for the test and fails on error.
func claim(t *testing.T, s *Store) uint64 {
	t.Helper()
	epoch, err := s.Claim()
	require.NoError(t, err)
	return epoch
}

// openStoreAt opens path with a fixed clock; the caller closes it.
func openStoreAt(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path, Options{Now: newFakeClock().Now})
	require.NoError(t, err)
	return s
}

// openClaimed opens a claimed store on clock that closes with the test.
func openClaimed(t *testing.T, clock *fakeClock) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"),
		Options{Now: clock.Now})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	claim(t, s)
	return s
}

// state is the state bucket as strings, the simplest typed view.
func state(s *Store) Keyed[string] {
	return StateValues[string](s)
}
