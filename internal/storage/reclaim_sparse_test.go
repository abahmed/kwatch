package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sparseFill leaves a file at least twice its data with little free
// space, as a long-running file's pages end up: it writes values of an
// eighth of a page under keys in no particular order, then deletes
// every other key. bbolt merges a page only when it is under a quarter
// full, so the pages stay part empty, and deleting a few keys per
// transaction lets the next one reuse the pages each frees. The data
// left is about data bytes; it returns a key that is kept.
func sparseFill(t *testing.T, s *Store, data int) string {
	t.Helper()
	value := strings.Repeat("v", s.bolt().Info().PageSize/8)
	keys := make([]string, 0, 2*data/len(value))
	items := map[string]Item[string]{}
	for i := 0; i < cap(keys); i++ {
		sum := sha256.Sum256([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		keys = append(keys, hex.EncodeToString(sum[:8]))
		items[keys[i]] = Item[string]{Value: value}
		if len(items) == 500 || i == cap(keys)-1 {
			require.NoError(t, state(s).PutAll(items))
			items = map[string]Item[string]{}
		}
	}
	for i := 1; i < len(keys); i += 2 * 50 {
		var odd []string
		for j := i; j < min(i+2*50, len(keys)); j += 2 {
			odd = append(odd, keys[j])
		}
		require.NoError(t, state(s).DeleteMany(odd))
	}
	return keys[0]
}

// withPageSize makes the files the test creates use pages of size
// bytes, whatever the OS's page size is.
func withPageSize(t *testing.T, size int) {
	t.Helper()
	old := newFilePageSize
	newFilePageSize = size
	t.Cleanup(func() { newFilePageSize = old })
}

// A file at least twice the size of the data the last pass measured,
// with little free space, is rewritten: its half-full pages cost memory
// when it is mapped. It runs with the 4 KiB pages of Linux on x86 and
// the 16 KiB pages of macOS on Apple silicon, whatever the test runs
// on: values of over half a 4 KiB page took one page each in the copy,
// so the copy gained nothing there and the test passed only on macOS.
func TestReclaimRewritesASparseFile(t *testing.T) {
	for _, page := range []int{4 << 10, 16 << 10} {
		t.Run(fmt.Sprintf("%d KiB pages", page>>10), func(t *testing.T) {
			withPageSize(t, page)
			reclaimsSparseFile(t, page)
		})
	}
}

func reclaimsSparseFile(t *testing.T, page int) {
	smallReclaim(t)
	s := openClaimed(t, newFakeClock())
	require.Equal(t, page, s.bolt().Info().PageSize)
	s.bolt().NoSync = true // the fill only; fsync is not under test
	kept := sparseFill(t, s, 6<<20)
	s.bolt().NoSync = false
	before, free := s.fileUsage()
	require.Less(t, free, before/2, "not a free-pages case")
	report, err := s.measure(context.Background())
	require.NoError(t, err)
	s.counters.logicalBytes.Store(report.total())
	t.Logf("file %d, data %d, free %d", before, report.total(), free)

	rewrote, err := s.Reclaim()

	require.NoError(t, err)
	assert.True(t, rewrote)
	after, _ := s.fileUsage()
	t.Logf("after %d", after)
	assert.Less(t, after, before*3/4, "the copy packs the pages")
	assert.Equal(t, page, s.bolt().Info().PageSize,
		"the copy keeps the file's page size")
	_, found, err := state(s).Get(kept)
	require.NoError(t, err)
	assert.True(t, found, "data kept")

	rewrote, err = s.Reclaim()
	require.NoError(t, err)
	assert.False(t, rewrote, "a packed file is left alone")
}

func TestReclaimDue(t *testing.T) {
	smallReclaim(t)
	mib := int64(1 << 20)
	for name, tc := range map[string]struct {
		size, free, logical, lastCopy int64
		want                          bool
	}{
		"mostly free":        {10 * mib, 6 * mib, 4 * mib, 0, true},
		"sparse":             {10 * mib, 0, 4 * mib, 0, true},
		"packed":             {10 * mib, 0, 8 * mib, 0, false},
		"sparse but tiny":    {mib, 0, mib / 4, 0, false},
		"not measured yet":   {10 * mib, 0, 0, 0, false},
		"healthy small free": {10 * mib, 2 * mib, 7 * mib, 0, false},
		"as big as the last copy left it": {
			10 * mib, 0, 4 * mib, 9 * mib, false},
		"outgrew the last copy": {
			20 * mib, 0, 4 * mib, 9 * mib, true},
	} {
		got := reclaimDue(tc.size, tc.free, tc.logical, tc.lastCopy)
		assert.Equal(t, tc.want, got, name)
	}
}

// A file of records about one page each stays twice its data after a
// copy: the next passes must not copy it again and again (found by a
// review). Such a copy is dropped, and the next passes do not copy
// again until the file outgrows it (TestReclaimDue).
func TestReclaimDoesNotRepeatACopyThatGainsNothing(t *testing.T) {
	smallReclaim(t)
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path, Options{Now: newFakeClock().Now})
	require.NoError(t, err)
	defer s.Close()
	claim(t, s)
	putPageRecords(t, s)
	c := NewCompactor(s, DefaultPolicy())
	swaps := 0
	for range 4 {
		before, _ := os.Stat(path)
		_, err := c.Pass(context.Background())
		require.NoError(t, err)
		after, _ := os.Stat(path)
		if !os.SameFile(before, after) {
			swaps++
		}
	}
	assert.Zero(t, swaps, "a copy that gained nothing replaced the file")
	assert.Zero(t, s.Stats().Rewrites)
	assert.Positive(t, s.lastCopy, "the copy's size is remembered")
}

// putPageRecords stores records of about one page each: a file that
// stays twice its data after a copy.
func putPageRecords(t *testing.T, s *Store) {
	t.Helper()
	items := map[string]Item[string]{}
	page := s.bolt().Info().PageSize
	for i := range 1500 {
		items[fmt.Sprintf("inc-%05d", i)] =
			Item[string]{Value: strings.Repeat("r", page-40)}
	}
	require.NoError(t, IncidentRecords[string](s).PutAll(items))
}

// The size a copy left is kept in the file: after a restart, a file
// that can't shrink is not copied again until it outgrows that size.
// A file from before it was kept reads as no copy measured.
func TestReclaimRemembersACopyAcrossRestarts(t *testing.T) {
	smallReclaim(t)
	path := filepath.Join(t.TempDir(), "state.db")
	clock := newFakeClock()
	s, err := Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	claim(t, s)
	putPageRecords(t, s)
	_, err = NewCompactor(s, DefaultPolicy()).Pass(context.Background())
	require.NoError(t, err)
	kept := s.lastCopy
	require.Positive(t, kept)
	require.True(t, s.copyNoGain, "the first pass did not try a copy")
	require.NoError(t, s.Close())

	s, err = Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	defer s.Close()
	claim(t, s)
	assert.Equal(t, kept, s.lastCopy, "the copy's size after a restart")
	_, err = NewCompactor(s, DefaultPolicy()).Pass(context.Background())
	require.NoError(t, err)
	assert.False(t, s.copyNoGain, "a restart copied the file again")
}

// A copy a crash left next to the file is removed when the file is
// claimed, before free space is measured: with a volume limit it would
// otherwise count as used and block every later rewrite.
func TestClaimRemovesALeftoverCopy(t *testing.T) {
	smallReclaim(t)
	path := filepath.Join(t.TempDir(), "state.db")
	clock := newFakeClock()
	s, err := Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	claim(t, s)
	require.NoError(t, s.Close())
	require.NoError(t, os.WriteFile(path+".compact", make([]byte, 8<<20),
		0o600))
	s, err = Open(path, Options{Now: clock.Now, VolumeLimit: 10 << 20})
	require.NoError(t, err)
	defer s.Close()
	claim(t, s)
	_, statErr := os.Stat(path + ".compact")
	assert.True(t, os.IsNotExist(statErr), "leftover copy still there")
}
