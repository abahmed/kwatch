package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
)

func plantFile(t *testing.T, path string, modified time.Time) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("copy"), 0o600))
	require.NoError(t, os.Chtimes(path, modified, modified))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// A command that was cut off leaves its copy of the state file behind.
// The next command removes it; it keeps a copy another command may still
// be reading, and every other file.
func TestCLIEditionRemovesCopiesLeftBehind(t *testing.T) {
	path := stateFileWith(t, feature.ProEdition, stateNow.Add(48*time.Hour))
	dir := filepath.Dir(path)
	stale := filepath.Join(dir, editionCopyPrefix+"111")
	fresh := filepath.Join(dir, editionCopyPrefix+"222")
	other := filepath.Join(dir, "notes.txt")
	plantFile(t, stale, stateNow.Add(-time.Hour))
	plantFile(t, fresh, stateNow.Add(-time.Minute))
	plantFile(t, other, stateNow.Add(-time.Hour))

	edition := NewCLIEdition(path, stateNow)

	require.Empty(t, edition.CommunitySends(alertsOf("discord", "slack")),
		"the edition is still read")
	require.False(t, exists(stale), "a copy left behind was kept")
	require.True(t, exists(fresh), "a copy in use was removed")
	require.True(t, exists(other), "an unrelated file was removed")
	require.True(t, exists(path), "the state file was removed")
	left, err := filepath.Glob(filepath.Join(dir, editionCopyPrefix+"*"))
	require.NoError(t, err)
	require.Equal(t, []string{fresh}, left, "the command left its own copy")
}

func TestSweepEditionCopiesToleratesAMissingDirectory(t *testing.T) {
	sweepEditionCopies(
		filepath.Join(t.TempDir(), "gone", "state.db"), stateNow)
}
