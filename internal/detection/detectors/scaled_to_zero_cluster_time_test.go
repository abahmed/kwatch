package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A scaled-to-zero Deployment whose EndpointSlices are not watched may
// have a twin serving its Service that kwatch cannot see: nothing is
// said until the kind syncs.
func TestWorkloadScaledToZeroWaitsForEndpointSliceSync(t *testing.T) {
	m, api := zeroShop()
	synced := func(k inventory.Kind) bool {
		return k != kube.KindEndpointSlice
	}

	got := evaluate(Workload{}, m, t0, api, synced)

	assert.Empty(t, got.Findings)
	assert.Equal(t, []inventory.Kind{kube.KindEndpointSlice}, got.Unsynced)
}

// The scale-down time comes from the cluster (the latest spec write the
// API server recorded), not from when kwatch first saw the workload.
func TestWorkloadScaledToZeroSinceComesFromTheCluster(t *testing.T) {
	m, api := zeroShop()
	written := t0.Add(-40 * 24 * time.Hour)
	put(m, api, t0.Add(-10*time.Minute), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
		kube.AttrSpecWritten:    inventory.Time(written),
	})

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.True(t, written.Equal(got[0].Since), got[0].Since.String())
	assert.NotContains(t, got[0].Summary, "before kwatch started")
}

func TestWorkloadScaledToZeroWithUnknownStartSaysSo(t *testing.T) {
	m, api := zeroShop()
	put(m, api, t0.Add(-10*time.Minute), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
	})

	got := detectZero(m, api)

	require.Len(t, got, 1)
	assert.Equal(t, "Is scaled to 0 (since before kwatch started) but "+
		"Ingress shop/web still routes traffic to it via Service api",
		got[0].Summary)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

// A scale-down that is news today becomes left-over after a day: the
// finding asks to be looked at again when it turns informational, so
// the flip is not waited for until something else changes.
func TestWorkloadScaledToZeroAsksToLookAgainWhenItTurnsInformational(
	t *testing.T,
) {
	m, api := zeroShop()
	put(m, api, t0.Add(-10*time.Minute), map[string]inventory.Value{
		kube.AttrReplicas:       inventory.Number(0),
		kube.AttrTemplateLabels: inventory.Text("app=api"),
		kube.AttrSpecWritten:    inventory.Time(t0.Add(-23 * time.Hour)),
	})

	got := evaluate(Workload{}, m, t0, api, nil)

	require.Len(t, got.Findings, 1)
	assert.Equal(t, detection.Warning, got.Findings[0].Severity)
	assert.Equal(t, time.Hour, got.RecheckAfter)

	later := evaluate(Workload{}, m, t0.Add(time.Hour), api, nil)
	require.Len(t, later.Findings, 1)
	assert.Equal(t, detection.Info, later.Findings[0].Severity)
}
