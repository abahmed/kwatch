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

// renewingLease returns a registry that saw the holder renew its Lease in
// a rhythm, and the time of the last scan.
func renewingLease(t *testing.T) (
	*detection.Registry, *inventory.Model, inventory.EntityID, time.Time,
) {
	m, lease := leaseModel(10*time.Second, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	require.Empty(t, scanLease(r, m, lease, leaseHolderName, t0, t0))
	last := t0.Add(kube.LeaseScanPeriod)
	require.Empty(t, scanLease(r, m, lease, leaseHolderName, last, last))
	return r, m, lease, last
}

// The Lease scan fails for five minutes (throttled) while the holder
// keeps renewing: the old renew time must not read as a stuck holder.
func TestLeaseStaysSilentWhileTheLeaseScanFails(t *testing.T) {
	r, m, lease, last := renewingLease(t)
	for at := last; at.Sub(last) <= 5*time.Minute; at = at.Add(30 *
		time.Second) {
		ev := r.Evaluate(m, at, lease)
		assert.Empty(t, ev.Findings, "at %s", at.Sub(last))
	}
	ev := r.Evaluate(m, last.Add(5*time.Minute), lease)
	assert.Equal(t, []string{"lease"}, ev.UnsyncedBy)
}

func TestLeaseStaysSilentWhileTheAPIServerIsDown(t *testing.T) {
	r, m, lease, last := renewingLease(t)
	at := last.Add(leaseStaleMin)
	put(m, kube.APIServer, at, map[string]inventory.Value{
		kube.AttrHealthy: inventory.Bool(false),
	})
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, last, at))
}

func TestLeaseKeepsItsFindingWhileTheLeaseScanFails(t *testing.T) {
	r, m, lease, last := renewingLease(t)
	at := last.Add(leaseStaleMin)
	found := scanLease(r, m, lease, leaseHolderName, last, at)
	require.Len(t, found, 1)

	ev := r.Evaluate(m, at.Add(5*time.Minute), lease)
	assert.Empty(t, ev.Findings)
	kept := ev.Keeping(found)
	require.Len(t, kept, 1, "a stale scan neither raises nor clears")
	assert.Equal(t, "LeaseStale", kept[0].Reason)
}

func TestLeaseReportsAStuckHolderOnceTheScanSucceedsAgain(t *testing.T) {
	r, m, lease, last := renewingLease(t)
	assert.Empty(t, r.Evaluate(m, last.Add(5*time.Minute), lease).Findings)
	found := scanLease(r, m, lease, leaseHolderName, last,
		last.Add(6*time.Minute))
	require.Len(t, found, 1)
	assert.Equal(t, "LeaseStale", found[0].Reason)
}
