package app

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/storage"
)

func captureKlog(t *testing.T) *bytes.Buffer {
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

// The claim line says where the start-up time went, step by step.
func TestOpenStoreLogsEveryStepOfTheClaim(t *testing.T) {
	useStateEnv(t)
	out := captureKlog(t)
	deps := epochDeps(leaseWithTransitions(ptr.To(int32(1))))

	s, err := openStore(context.Background(), deps)
	require.NoError(t, err)
	defer closeStore(s)
	klog.Flush()

	logged := out.String()
	assert.Contains(t, logged, `"claimed state store"`)
	for _, field := range []string{"openMs=", "leaseCheckMs=",
		"claimMs=", "totalMs=", "fileBytes="} {
		assert.Contains(t, logged, field)
	}
	assert.NotContains(t, logged, "was slow")
}

// steppingClock moves forward by step on every reading.
type steppingClock struct {
	clock.RealClock
	now  time.Time
	step time.Duration
}

func (c *steppingClock) Now() time.Time {
	c.now = c.now.Add(c.step)
	return c.now
}

// A step that takes longer than storage.SlowStep is named on its own.
func TestOpenStoreNamesASlowStep(t *testing.T) {
	useStateEnv(t)
	out := captureKlog(t)
	deps := epochDeps(leaseWithTransitions(ptr.To(int32(1))))
	deps.clients.Clock = &steppingClock{
		now:  time.Date(2026, 10, 6, 17, 0, 0, 0, time.UTC),
		step: 2 * storage.SlowStep,
	}

	s, err := openStore(context.Background(), deps)
	require.NoError(t, err)
	defer closeStore(s)
	klog.Flush()

	assert.Contains(t, out.String(), `"state startup step was slow"`)
	assert.Contains(t, out.String(), `step="lease check"`)
}
