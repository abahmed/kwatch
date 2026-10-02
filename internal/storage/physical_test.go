package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// growFile appends n log entries of 4 KiB to s.
func growFile(t *testing.T, s *Store, n int) {
	t.Helper()
	big := strings.Repeat("x", 4096)
	entries := make([]Entry[string], n)
	for i := range entries {
		entries[i] = Entry[string]{
			Entity: fmt.Sprintf("pod-%04d", i), At: minute(i), Value: big,
		}
	}
	require.NoError(t, ChangeLog[string](s).AppendAll(entries))
}

func TestPhysicalTargetLowersCapOnlyForClearExcess(t *testing.T) {
	const sizeCap = 8 << 20
	allowed := int64(sizeCap + sizeCap/compactMarginDivisor)
	assert.Equal(t, int64(sizeCap), physicalTarget(sizeCap, 0))
	assert.Equal(t, int64(sizeCap), physicalTarget(sizeCap, allowed))
	assert.Equal(t, int64(sizeCap-1000),
		physicalTarget(sizeCap, allowed+1000))
	assert.Equal(t, int64(1000), physicalTarget(1000, fileOverhead),
		"bbolt's own pages are not growth")
}

// A file clearly over the cap lowers the logical target, so freed pages
// are reused instead of the file growing, and asks for a rewrite.
func TestCompactorEnforcesCapOnFileSize(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	growFile(t, s, 600)
	fileBytes, err := fileSize(s.path)
	require.NoError(t, err)
	policy := Policy{SizeCap: 512 << 10, Batch: 100}
	require.Greater(t, fileBytes, physicalTarget(policy.SizeCap, 0)+
		fileOverhead+policy.SizeCap/compactMarginDivisor)

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.LessOrEqual(t, result.Bytes, policy.SizeCap/minHistoryDivisor+
		entrySize(t, "pod-0000", strings.Repeat("x", 4096)),
		"the file's excess lowered the target to the history floor")
	assert.Equal(t, fileBytes, result.FileBytes, "bbolt keeps its pages")
	assert.True(t, result.RewriteDue)
	assert.True(t, s.Stats().RewriteDue)
}

func TestCompactorWithinCapNeedsNoRewrite(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	growFile(t, s, 10)

	result, err := NewCompactor(s, Policy{}).Pass(context.Background())

	require.NoError(t, err)
	assert.False(t, result.RewriteDue)
	assert.False(t, s.Stats().RewriteDue)
}

// An emptyDir's sizeLimit is smaller than the node disk the file system
// reports. The startup rewrite must not fill the volume past it.
func TestOpenSkipsRewriteBeyondVolumeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	fillAndDelete(t, path, 200)
	before := fileBytes(t, path)

	s, err := Open(path, Options{
		Now: newFakeClock().Now, SizeCap: 64 << 10,
		FreeSpace:   func(string) (uint64, error) { return 1 << 40, nil },
		VolumeLimit: before + 4096,
	})
	require.NoError(t, err)
	defer s.Close()

	assert.Equal(t, before, fileBytes(t, path), "no room for the copy")
}

func TestVolumeLimitBoundsFreeSpace(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"),
		make([]byte, 1000), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b"),
		make([]byte, 500), 0o600))
	disk := func(string) (uint64, error) { return 1 << 40, nil }
	path := filepath.Join(dir, "state.db")

	limited := Options{FreeSpace: disk, VolumeLimit: 4000}
	free, err := freeSpaceOf(limited)(path)
	require.NoError(t, err)
	assert.Equal(t, uint64(2500), free)

	free, err = freeSpaceOf(Options{FreeSpace: disk, VolumeLimit: 100})(path)
	require.NoError(t, err)
	assert.Zero(t, free, "a volume over its limit has no room")

	free, err = freeSpaceOf(Options{FreeSpace: disk})(path)
	require.NoError(t, err)
	assert.Equal(t, uint64(1<<40), free, "no limit trusts the disk")
}
