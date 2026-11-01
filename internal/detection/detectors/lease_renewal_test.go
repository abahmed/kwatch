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

// scanLeaseOf records one scan of a Lease of the given duration in seconds
// and evaluates it.
func scanLeaseOf(
	r *detection.Registry, m *inventory.Model, lease inventory.EntityID,
	holder string, seconds float64, renewed, now time.Time,
) []detection.Finding {
	put(m, lease, now, map[string]inventory.Value{
		kube.AttrLeaseHolder:   inventory.Text(holder),
		kube.AttrLeaseRenewed:  inventory.Time(renewed),
		kube.AttrLeaseDuration: inventory.Number(seconds),
	})
	return r.Evaluate(m, now, lease).Findings
}

// scanLease records one scan of a 15 second Lease and evaluates it.
func scanLease(
	r *detection.Registry, m *inventory.Model, lease inventory.EntityID,
	holder string, renewed, now time.Time,
) []detection.Finding {
	return scanLeaseOf(r, m, lease, holder, 15, renewed, now)
}

func TestLeaseStaysSilentForOneShotLeaseOfHealthyHolder(t *testing.T) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	taken := t0.Add(-time.Hour)

	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, taken, t0))
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, taken,
		t0.Add(24*time.Hour)), "a Lease never seen renewing stays silent")
}

func TestLeaseBumpedOnlyOnRestartStaysQuietForTheSameHolder(t *testing.T) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName,
		t0.Add(-10*time.Hour), t0))

	// Hours later the container restarts and the same identity takes the
	// Lease again: one slow bump is no rhythm.
	again := t0.Add(kube.LeaseScanPeriod)
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, again, again))
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, again,
		again.Add(24*time.Hour)))
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

func TestLeaseShorterThanTheScanPeriodIsStillDetected(t *testing.T) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	last := t0.Add(kube.LeaseScanPeriod)

	// Three durations are 15s: far less than the time between two scans.
	assert.Empty(t, scanLeaseOf(r, m, lease, leaseHolderName, 5, t0, t0))
	assert.Empty(t, scanLeaseOf(r, m, lease, leaseHolderName, 5, last, last))

	found := scanLeaseOf(r, m, lease, leaseHolderName, 5, last,
		last.Add(leaseStaleMin))
	require.Len(t, found, 1)
}

func TestLeaseOfLongDurationIsDetectedAfterThreeDurations(t *testing.T) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	const seconds = 3600
	last := t0.Add(50 * time.Minute)

	assert.Empty(t, scanLeaseOf(r, m, lease, leaseHolderName, seconds,
		t0, t0))
	assert.Empty(t, scanLeaseOf(r, m, lease, leaseHolderName, seconds,
		last, last))
	assert.Empty(t, scanLeaseOf(r, m, lease, leaseHolderName, seconds,
		last, last.Add(2*time.Hour+59*time.Minute)))

	found := scanLeaseOf(r, m, lease, leaseHolderName, seconds, last,
		last.Add(3*time.Hour))
	require.Len(t, found, 1)
}

func TestLeaseRenewalGapAllowanceIsTwoScansForShortLeases(t *testing.T) {
	tests := []struct {
		name string
		gap  time.Duration
		want int
	}{
		{"at the allowance", 2 * kube.LeaseScanPeriod, 1},
		{"past the allowance", 2*kube.LeaseScanPeriod + time.Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
			r := detection.NewRegistry(nil, Lease{})
			last := t0.Add(tt.gap)
			scanLease(r, m, lease, leaseHolderName, t0, t0)
			scanLease(r, m, lease, leaseHolderName, last, last)

			found := scanLease(r, m, lease, leaseHolderName, last,
				last.Add(24*time.Hour))

			assert.Len(t, found, tt.want)
		})
	}
}

// Once a holder was seen to renew in rhythm it stays proven: a late
// renewal after a long stall is a holder that stalled, and silence then
// would hide the stall that matters.
func TestLeaseRhythmSurvivesALateRenewal(t *testing.T) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	r := detection.NewRegistry(nil, Lease{})
	second := t0.Add(kube.LeaseScanPeriod)
	late := second.Add(10 * time.Hour)
	scanLease(r, m, lease, leaseHolderName, t0, t0)
	scanLease(r, m, lease, leaseHolderName, second, second)
	assert.Empty(t, scanLease(r, m, lease, leaseHolderName, late, late))

	found := scanLease(r, m, lease, leaseHolderName, late,
		late.Add(leaseStaleMin))

	assert.Len(t, found, 1)
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

func TestLeaseAfterKwatchRestartStaysQuietUntilARhythmIsSeen(t *testing.T) {
	m, lease := leaseModel(time.Hour, leaseHolderName, "Running")
	before := detection.NewRegistry(nil, Lease{})
	last := t0.Add(kube.LeaseScanPeriod)
	scanLease(before, m, lease, leaseHolderName, t0, t0)
	scanLease(before, m, lease, leaseHolderName, last, last)

	// A new process remembers nothing: the stalled Lease is not reported.
	after := detection.NewRegistry(nil, Lease{})
	later := last.Add(24 * time.Hour)
	assert.Empty(t, scanLease(after, m, lease, leaseHolderName, last, later))

	first := later.Add(time.Minute)
	second := first.Add(kube.LeaseScanPeriod)
	assert.Empty(t, scanLease(after, m, lease, leaseHolderName, first, first))
	assert.Empty(t, scanLease(after, m, lease, leaseHolderName, second,
		second))
	assert.Len(t, scanLease(after, m, lease, leaseHolderName, second,
		second.Add(leaseStaleMin)), 1, "a rhythm seen again counts")
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
