package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"
)

// The pass line names every bucket's bytes and how far back the timeline
// reaches, so a growing bucket and the real timeline window are visible
// without opening the file.
func TestPassLogsEveryBucketAndTheOldestTimelineEntry(t *testing.T) {
	out := captureLog(t)
	clock := newFakeClock()
	s := openClaimed(t, clock)
	require.NoError(t, TimelineLog[string](s).Append("pod/a", clock.Now().
		Add(-3*time.Hour), "old"))
	require.NoError(t, TimelineLog[string](s).Append("pod/b", clock.Now().
		Add(-time.Hour), "new"))
	require.NoError(t, BaselineValues[string](s).Put("k", "v"))

	res, err := NewCompactor(s, DefaultPolicy()).Pass(context.Background())
	klog.Flush()

	require.NoError(t, err)
	assert.Equal(t, 3*time.Hour, res.OldestTimeline.Round(time.Minute))
	logged := out.String()
	for _, sp := range specs {
		assert.Contains(t, logged, string(sp.name)+"Bytes=", sp.name)
	}
	assert.Contains(t, logged, "oldestTimelineAge=")
	assert.Contains(t, logged, `"3h0m0s"`)
}
