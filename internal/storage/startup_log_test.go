package storage

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"
)

// captureLog returns what klog writes until the test ends.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	return &out
}

// slowClock moves forward by step on every reading.
func slowClock(t *testing.T, step time.Duration) {
	t.Helper()
	now := time.Date(2026, 10, 6, 17, 0, 0, 0, time.UTC)
	startupNow = func() time.Time {
		now = now.Add(step)
		return now
	}
	t.Cleanup(func() { startupNow = time.Now })
}

func TestOpenLogsTheFileSizeVersionAndDuration(t *testing.T) {
	out := captureLog(t)
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)

	s := openDeferred(t, path, Options{})
	klog.Flush()

	require.NotNil(t, s)
	logged := out.String()
	assert.Contains(t, logged, `"state file opened"`)
	assert.Contains(t, logged, "fileBytes=")
	assert.Contains(t, logged, "schemaVersion=2")
	assert.Contains(t, logged, "durationMs=")
}

func TestOpenLogsAStepThatTakesLongerThanSlowStep(t *testing.T) {
	out := captureLog(t)
	slowClock(t, 2*SlowStep)
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)

	openDeferred(t, path, Options{})
	klog.Flush()

	logged := out.String()
	assert.Contains(t, logged, `"state startup step was slow"`)
	assert.Contains(t, logged, `step="page check"`)
}

func TestFastStartupStepsAreNotLoggedAsSlow(t *testing.T) {
	out := captureLog(t)
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)

	openDeferred(t, path, Options{})
	klog.Flush()

	assert.NotContains(t, out.String(), "was slow")
}
