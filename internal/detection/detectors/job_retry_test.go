package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var retryNow = time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)

// retryingJobRun is jobRun with the owning Job's retry counts: failed
// pods so far and the backoff limit (0 leaves it unset, so the default).
func retryingJobRun(failed, limit float64, ending string,
) (*inventory.Model, inventory.Entity, inventory.Entity) {
	model, pod, container := jobRun("running")
	job := newID(kube.KindJob, "ns", "j")
	attrs := map[string]inventory.Value{
		kube.AttrFailed: inventory.Number(failed),
	}
	if limit > 0 {
		attrs[kube.AttrBackoffLimit] = inventory.Number(limit)
	}
	put(model, job, retryNow, attrs)
	if ending != "" {
		setCondition(model, job, ending, "True", "", "", retryNow)
	}
	return model, pod, container
}

func jobRunFindings(
	model *inventory.Model, pod, container inventory.Entity,
) (podFound, containerFound []detection.Finding) {
	ctx := testDetectorContext(model, retryNow)
	return NewPod(PodThresholds{}).Detect(ctx, pod),
		Container{}.Detect(ctx, container)
}

// A Job pod that fails while the Job has retries left is the Job doing
// its work: it is advice until half of the retries are spent.
func TestJobPodFailureWithRetriesLeftIsAdvisory(t *testing.T) {
	for _, failed := range []float64{0, 1, 2} {
		model, pod, container := retryingJobRun(failed, 0, "")

		podFound, containerFound := jobRunFindings(model, pod, container)

		require.Len(t, podFound, 1, failed)
		require.Len(t, containerFound, 1, failed)
		assert.Equal(t, reasons.PodFailed, podFound[0].Reason)
		assert.Equal(t, reasons.Error, containerFound[0].Reason)
		assert.True(t, podFound[0].Advisory, "pod, failed=%v", failed)
		assert.True(t, containerFound[0].Advisory,
			"container, failed=%v", failed)
	}
}

func TestJobPodFailureIsRealOnceHalfTheRetriesAreSpent(t *testing.T) {
	for _, failed := range []float64{3, 4, 5} {
		model, pod, container := retryingJobRun(failed, 0, "")

		podFound, containerFound := jobRunFindings(model, pod, container)

		require.Len(t, podFound, 1, failed)
		require.Len(t, containerFound, 1, failed)
		assert.False(t, podFound[0].Advisory, "pod, failed=%v", failed)
		assert.False(t, containerFound[0].Advisory,
			"container, failed=%v", failed)
	}
}

func TestJobPodFailureUsesTheJobsOwnBackoffLimit(t *testing.T) {
	model, pod, container := retryingJobRun(3, 20, "")
	podFound, containerFound := jobRunFindings(model, pod, container)
	assert.True(t, podFound[0].Advisory)
	assert.True(t, containerFound[0].Advisory)

	model, pod, container = retryingJobRun(10, 20, "")
	podFound, containerFound = jobRunFindings(model, pod, container)
	assert.False(t, podFound[0].Advisory)
	assert.False(t, containerFound[0].Advisory)
}

func TestJobPodFailureOfAFailedJobIsNeverAdvisory(t *testing.T) {
	model, pod, container := retryingJobRun(1, 0, "Failed")

	podFound, containerFound := jobRunFindings(model, pod, container)

	assert.False(t, podFound[0].Advisory)
	assert.False(t, containerFound[0].Advisory)
}

func TestStandaloneFailedPodIsNeverAdvisory(t *testing.T) {
	model, pod, container := jobRun("")

	podFound, containerFound := jobRunFindings(model, pod, container)

	assert.False(t, podFound[0].Advisory)
	assert.False(t, containerFound[0].Advisory)
}
