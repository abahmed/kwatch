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

const leaseHolderName = "operator-7d9f_1a2b"

// scanLease records one scan of the Lease and evaluates it.
func scanLease(
	r *detection.Registry, m *inventory.Model, lease inventory.EntityID,
	holder string, renewed, now time.Time,
) []detection.Finding {
	put(m, lease, now, map[string]inventory.Value{
		kube.AttrLeaseHolder:   inventory.Text(holder),
		kube.AttrLeaseRenewed:  inventory.Time(renewed),
		kube.AttrLeaseDuration: inventory.Number(15),
	})
	return r.Evaluate(m, now, lease).Findings
}

func TestLeaseStaysSilentForOneShotLeaseOfHealthyHolder(t *testing.T) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	taken := t0.Add(-time.Hour)

	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, taken, t0))
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, taken,
		t0.Add(24*time.Hour)), "a Lease never seen renewing stays silent")
}

func TestLeaseReportsHealthyHolderSeenToRenewThenStop(t *testing.T) {
	m, lease := leaseModel(10*time.Second, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})

	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, t0, t0))
	last := t0.Add(kube.LeaseScanPeriod)
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, last, last))

	found := scanLease(r, m, lease, leaseHolderName, last,
		last.Add(leaseStaleMin))
	require.Len(t, found, 1)
	assert.Equal(t, "LeaseStale", found[0].Reason)
}

func TestLeaseNewHolderMustBeSeenToRenewItself(t *testing.T) {
	m, lease := leaseModel(10*time.Second, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	last := t0.Add(kube.LeaseScanPeriod)
	scanLease(r, m, lease, leaseHolderName, t0, t0)
	scanLease(r, m, lease, leaseHolderName, last, last)

	const next = "operator-7d9f_ffff"
	taken := last.Add(time.Minute)
	assert.Empty(t, scanLease(r, m, lease, next, taken, taken))
	assert.Empty(t, scanLease(r, m, lease, next, taken, taken.Add(time.Hour)),
		"the renewal belonged to the previous holder")
}

func TestLeaseNeverSeenRenewingIsReportedWhileHolderIsUnhealthy(
	t *testing.T,
) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	put(m, newID(kube.KindPod, "ops", "operator-7d9f"), t0,
		map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
			kube.AttrReady: inventory.Bool(false),
		})
	r := detection.NewRegistry(nil, Lease{})

	found := scanLease(r, m, lease, leaseHolderName, t0.Add(-time.Hour), t0)
	require.Len(t, found, 1)
	assert.Contains(t, found[0].Summary, "not ready")
}
