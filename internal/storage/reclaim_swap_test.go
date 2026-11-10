package storage

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// When the copy cannot be opened, the rewrite gives up and the store
// keeps the original: writes and reads go on as before.
func TestReclaimKeepsTheFileWhenTheCopyWontOpen(t *testing.T) {
	smallReclaim(t)
	s := openClaimed(t, newFakeClock())
	fillAndEmpty(t, s, 1500)
	require.NoError(t, state(s).Put("kept", "v"))
	old := openCopy
	openCopy = func(string) (*bolt.DB, error) {
		return nil, errors.New("cannot open")
	}
	t.Cleanup(func() { openCopy = old })

	rewrote, err := s.Reclaim()

	require.NoError(t, err)
	assert.False(t, rewrote)
	require.NoError(t, state(s).Put("after", "v"))
	v, found, err := state(s).Get("kept")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "v", v)
}

// A read that is still running when the rewrite swaps files does not
// hold up reads that start meanwhile: they use the copy, and the old
// file closes once the running read ends.
func TestReclaimSwapDoesNotBlockNewReads(t *testing.T) {
	smallReclaim(t)
	s := openClaimed(t, newFakeClock())
	fillAndEmpty(t, s, 1500)
	require.NoError(t, state(s).Put("kept", "v"))
	first := s.bolt()
	inRead, release := make(chan struct{}), make(chan struct{})
	readDone := make(chan error, 1)
	var once sync.Once
	end := func() { once.Do(func() { close(release) }) }
	defer end() // a failing check must not leave the read open
	go func() {
		readDone <- s.view(func(*bolt.Tx) error {
			close(inRead)
			<-release
			return nil
		})
	}()
	<-inRead
	reclaimed := make(chan bool, 1)
	go func() {
		rewrote, err := s.Reclaim()
		assert.NoError(t, err)
		reclaimed <- rewrote
	}()
	require.Eventually(t, func() bool { return s.bolt() != first },
		5*time.Second, 5*time.Millisecond, "the swap never happened")

	got := make(chan bool, 1)
	go func() {
		_, found, err := state(s).Get("kept")
		assert.NoError(t, err)
		got <- found
	}()
	select {
	case found := <-got:
		assert.True(t, found)
	case <-time.After(2 * time.Second):
		end()
		t.Fatal("a new read waited for the running one")
	}
	end()
	require.NoError(t, <-readDone)
	assert.True(t, <-reclaimed)
}

// A copy that never went live must not set the size the next rewrite
// waits for, or one failed open delays the retry.
func TestReclaimFailedOpenKeepsTheLastCopySize(t *testing.T) {
	smallReclaim(t)
	s := openClaimed(t, newFakeClock())
	fillAndEmpty(t, s, 1500)
	before := s.lastCopy
	old := openCopy
	openCopy = func(string) (*bolt.DB, error) {
		return nil, errors.New("cannot open")
	}
	t.Cleanup(func() { openCopy = old })

	rewrote, err := s.Reclaim()

	require.NoError(t, err)
	assert.False(t, rewrote)
	assert.Equal(t, before, s.lastCopy)
}
