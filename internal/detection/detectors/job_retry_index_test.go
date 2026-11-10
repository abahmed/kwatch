package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An indexed Job with a retry limit per index counts failures across
// every index, so many failed pods still leave each index its retries:
// a failed pod stays advice until the Job itself fails.
func TestIndexedJobRetriesEachIndexOnItsOwn(t *testing.T) {
	model, pod, container := retryingJobRun(5, 0, "")
	job := newID(kube.KindJob, "ns", "j")
	put(model, job, retryNow, map[string]inventory.Value{
		kube.AttrFailed:               inventory.Number(5),
		kube.AttrBackoffLimitPerIndex: inventory.Number(2),
	})

	podFound, containerFound := jobRunFindings(model, pod, container)

	require.Len(t, podFound, 1)
	require.Len(t, containerFound, 1)
	assert.True(t, podFound[0].Advisory)
	assert.True(t, containerFound[0].Advisory)
}
