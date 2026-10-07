package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// smallReclaim lets tests rewrite files of a few megabytes.
func smallReclaim(t *testing.T) {
	t.Helper()
	old := reclaimMin
	reclaimMin = 1 << 20
	t.Cleanup(func() { reclaimMin = old })
}

// legacyFile writes what the release before this one left on disk:
// thirty days of timeline that was never trimmed, open incidents,
// resolved ones and fingerprints. It returns the clock at the end.
func legacyFile(t *testing.T, path string) *fakeClock {
	t.Helper()
	clock := newFakeClock()
	s, err := Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	claim(t, s)
	text := strings.Repeat("t", churnTimelineEntryText)
	start := clock.Now().Add(-30 * day)
	for quarter := 0; quarter < 30*4; quarter++ {
		entries := make([]Entry[string], 900)
		for i := range entries {
			entries[i] = Entry[string]{
				Entity: fmt.Sprintf("pod/ns/e-%03d", i%150),
				At: start.Add(time.Duration(quarter)*6*time.Hour +
					time.Duration(i)*time.Second),
				Value: text,
			}
		}
		require.NoError(t, TimelineLog[string](s).AppendAll(entries))
	}
	incidents := map[string]Item[string]{}
	for i := 0; i < 550; i++ {
		item := Item[string]{Value: strings.Repeat("r", 3000)} // open
		if i%2 == 0 {
			// Resolved, still remembered.
			item.Expires = clock.Now().Add(3 * day)
		}
		incidents[fmt.Sprintf("inc-%03d", i)] = item
	}
	require.NoError(t, IncidentRecords[string](s).PutAll(incidents))
	prints := map[string]Item[string]{}
	for i := 0; i < churnFingerprints; i++ {
		prints[fmt.Sprintf("pod/ns/p-%d", i)] = Item[string]{Value: "d"}
	}
	require.NoError(t, FingerprintValues[string](s).PutAll(prints))
	require.NoError(t, s.Close())
	return clock
}

// The file an older release left behind opens with the new code and the
// first compactor pass trims it and gives the space back, keeping every
// incident and fingerprint.
func TestLegacyFileIsTrimmedAndRewrittenByTheFirstPass(t *testing.T) {
	smallReclaim(t)
	path := filepath.Join(t.TempDir(), "state.db")
	clock := legacyFile(t, path)
	before, _ := fileSize(path)

	s, err := Open(path, Options{Now: clock.Now, DeferRepair: true})
	require.NoError(t, err)
	defer s.Close()
	claim(t, s)
	result, err := NewCompactor(s, DefaultPolicy()).Pass(context.Background())

	require.NoError(t, err)
	after, _ := fileSize(path)
	t.Logf("legacy file %d bytes -> %d bytes (expired=%d evicted=%d)",
		before, after, result.Expired, result.Evicted)
	assert.True(t, result.Rewrote)
	assert.Less(t, after, before/3)
	assert.Equal(t, after, result.FileBytes)
	sizes, err := s.LogicalSize()
	require.NoError(t, err)
	assert.LessOrEqual(t, sizes[Timeline], DefaultTimelineCap)
	kept := 0
	require.NoError(t, IncidentRecords[string](s).Range("",
		func(string, string) error { kept++; return nil }))
	assert.Equal(t, 550, kept, "no incident is lost")
	prints := 0
	require.NoError(t, FingerprintValues[string](s).Range("",
		func(string, string) error { prints++; return nil }))
	assert.Equal(t, churnFingerprints, prints)
	assert.Equal(t, uint64(1), s.Stats().Rewrites)
	require.NoError(t, state(s).Put("after", "swap"),
		"the claim survives the swap and writes continue")
}

// Readers and writers that run while the file is rewritten neither fail
// nor lose a value.
func TestReclaimUnderConcurrentUse(t *testing.T) {
	smallReclaim(t)
	clock := newFakeClock()
	s := openClaimed(t, clock)
	fillAndEmpty(t, s, 1500)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []error
	fail := func(err error) {
		mu.Lock()
		failures = append(failures, err)
		mu.Unlock()
	}
	wrote := 0
	wg.Add(2)
	go func() {
		defer wg.Done()
		for ; ; wrote++ {
			select {
			case <-stop:
				return
			default:
			}
			if err := state(s).Put(fmt.Sprintf("new-%06d", wrote), "v"); err != nil {
				fail(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, _, err := state(s).Get("new-000000"); err != nil {
				fail(err)
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)
	rewrote, err := s.Reclaim()
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()

	require.NoError(t, err)
	assert.True(t, rewrote)
	assert.Empty(t, failures)
	for i := 0; i < wrote; i++ {
		_, found, err := state(s).Get(fmt.Sprintf("new-%06d", i))
		require.NoError(t, err)
		require.True(t, found, "value %d was lost", i)
	}
}

func TestReclaimLeavesAHealthyFileAlone(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	require.NoError(t, state(s).Put("k", "v"))

	rewrote, err := s.Reclaim()

	require.NoError(t, err)
	assert.False(t, rewrote)
}

func TestReclaimRefusesAnUnclaimedOrDeposedHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStoreAt(t, path)
	defer s.Close()
	_, err := s.Reclaim()
	assert.ErrorIs(t, err, ErrNotClaimed)
	claim(t, s)
	_, err = s.ClaimNew()
	require.NoError(t, err)

	_, err = s.Reclaim()

	assert.ErrorIs(t, err, ErrFenced)
}

func TestReclaimSkipsWithoutRoomForTheCopy(t *testing.T) {
	smallReclaim(t)
	path := filepath.Join(t.TempDir(), "state.db")
	clock := newFakeClock()
	s, err := Open(path, Options{Now: clock.Now,
		FreeSpace: func(string) (uint64, error) { return 1 << 20, nil }})
	require.NoError(t, err)
	defer s.Close()
	claim(t, s)
	fillAndEmpty(t, s, 1500)

	rewrote, err := s.Reclaim()

	require.NoError(t, err)
	assert.False(t, rewrote)
}

// fillAndEmpty leaves n large values' worth of free pages in the file.
func fillAndEmpty(t *testing.T, s *Store, n int) {
	t.Helper()
	items := make(map[string]Item[string], n)
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("old-%04d", i)
		items[key] = Item[string]{Value: strings.Repeat("x", 4096)}
		keys = append(keys, key)
	}
	require.NoError(t, state(s).PutAll(items))
	require.NoError(t, state(s).DeleteMany(keys))
}
