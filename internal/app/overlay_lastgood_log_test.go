package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	berrors "go.etcd.io/bbolt/errors"
	"k8s.io/klog/v2"
)

// The reason an invalid KwatchConfig starts with no earlier spec is worth
// reading at the default verbosity: it says why the silences and filters
// of the last good spec are missing.
func TestReadLastGoodSaysWhyAnUnreadableStateFileGivesNoSpec(t *testing.T) {
	const notUsed = "cannot use the last good KwatchConfig"
	out := captureKlog(t)
	path := filepath.Join(t.TempDir(), "state.db")
	require.NoError(t, os.WriteFile(path, make([]byte, 20000), 0o600))

	_, ok := readLastGood(path, time.Now)
	klog.Flush()

	require.False(t, ok)
	text := out.String()
	require.Equal(t, 1, strings.Count(text, notUsed), text)
	require.Regexp(t, `(?m)^I\d+ .*`+notUsed, text, "Info, not verbose")
	require.Contains(t, text, `reason="state file cannot be read"`)
	require.Contains(t, text, "error=")
}

func TestReadLastGoodSaysWhenTheKeptSpecIsNotJSON(t *testing.T) {
	const notUsed = "cannot use the last good KwatchConfig"
	out := captureKlog(t)
	path := filepath.Join(t.TempDir(), "state.db")
	disk := stateAt(t, path)
	require.NoError(t, disk.put(stateOverlayLastGood, "{not json"))
	require.NoError(t, disk.store.Close())

	_, ok := readLastGood(path, time.Now)
	klog.Flush()

	require.False(t, ok)
	text := out.String()
	require.Equal(t, 1, strings.Count(text, notUsed), text)
	require.Contains(t, text, `reason="kept spec is not valid JSON"`)
}

func TestReadLastGoodSaysNothingWhenNoSpecWasKept(t *testing.T) {
	const notUsed = "cannot use the last good KwatchConfig"
	out := captureKlog(t)
	path := filepath.Join(t.TempDir(), "state.db")
	disk := stateAt(t, path)
	require.NoError(t, disk.put(stateOverlayLastGood, ""))
	require.NoError(t, disk.store.Close())

	_, ok := readLastGood(path, time.Now)
	klog.Flush()

	require.False(t, ok)
	require.NotContains(t, out.String(), notUsed,
		"a deleted KwatchConfig is a choice, not a failure")
}

func TestLastGoodOpenReasonNamesALockTimeout(t *testing.T) {
	require.Equal(t, "state file is locked by another process",
		lastGoodOpenReason(fmt.Errorf("open: %w", berrors.ErrTimeout)))
	require.Equal(t, "state file cannot be opened",
		lastGoodOpenReason(errors.New("permission denied")))
}
