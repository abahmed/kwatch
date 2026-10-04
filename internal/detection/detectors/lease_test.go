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

// leaseModel builds a Lease renewed age ago by holder, and the holder's
// pod when podPhase is not empty.
func leaseModel(
	age time.Duration, holder, podPhase string,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	lease := newID(kube.KindLease, "ops", "operator-leader")
	put(m, lease, t0, map[string]inventory.Value{
		kube.AttrLeaseHolder:   inventory.Text(holder),
		kube.AttrLeaseRenewed:  inventory.Time(t0.Add(-age)),
		kube.AttrLeaseDuration: inventory.Number(15),
	})
	if podPhase != "" {
		put(m, newID(kube.KindPod, "ops", "operator-7d9f"), t0,
			map[string]inventory.Value{kube.AttrPhase: inventory.Text(podPhase)})
	}
	return m, lease
}

func TestLeaseStaleWhileHolderRuns(t *testing.T) {
	m, lease := leaseModel(5*time.Minute, "operator-7d9f_1a2b", "Running")

	got := Lease{}.Detect(testDetectorContext(m, t0), entityOf(m, lease))

	require.Len(t, got, 1)
	assert.Equal(t, "LeaseStale", got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "pod operator-7d9f")
}

func TestLeaseQuietWhenFreshOrHolderGone(t *testing.T) {
	fresh, lease := leaseModel(10*time.Second, "operator-7d9f_1a2b",
		"Running")
	assert.Empty(t, Lease{}.Detect(testDetectorContext(fresh, t0),
		entityOf(fresh, lease)))

	gone, lease := leaseModel(time.Hour, "operator-7d9f_1a2b", "")
	assert.Empty(t, Lease{}.Detect(testDetectorContext(gone, t0),
		entityOf(gone, lease)), "an uninstalled controller leaves a lease")

	done, lease := leaseModel(time.Hour, "operator-7d9f_1a2b", "Succeeded")
	assert.Empty(t, Lease{}.Detect(testDetectorContext(done, t0),
		entityOf(done, lease)))
}
