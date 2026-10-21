package incident

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

// restoredNodePage announces the page of node n1, restarts kwatch ten
// minutes later and, with ready true, shows the node ready again since
// readyAt, as the first sync of the new session does.
func restoredNodePage(
	t *testing.T, readyAt time.Duration, ready bool,
) (*rig, time.Time) {
	t.Helper()
	return restoredNodeIncident(t, nodeDown(), readyAt, ready)
}

// restoredNodeIncident is restoredNodePage for any failure of node n1.
func restoredNodeIncident(
	t *testing.T, failure detection.Finding, readyAt time.Duration,
	ready bool,
) (*rig, time.Time) {
	t.Helper()
	old := newRig(t, Config{})
	old.raise(at(0), failure)
	require.Len(t, tickEvery(old, 0, 2*DefaultSettle), 1)

	restartAt := at(10 * time.Minute)
	fresh := newRig(t, Config{})
	fresh.model = old.model
	fresh.setAttrs(nodeID(), map[string]inventory.Value{
		kube.AttrReady:      inventory.Bool(ready),
		kube.AttrReadySince: inventory.Time(at(readyAt)),
	})
	fresh.m.Restore(old.m.Export(), restartAt.Add(10*time.Minute))
	// The first solve of the session hands the manager the model.
	fresh.m.Apply(fresh.snapshot(restartAt), nil, nil)
	return fresh, restartAt
}

// A node that came back while kwatch was down resolves without waiting
// for the restore grace and the hold: it is already healthy, and has been
// for longer than the hold.
func TestRestoredNodeThatRecoveredWhileDownResolvesAtOnce(t *testing.T) {
	r, restartAt := restoredNodePage(t, 4*time.Minute, true)

	var resolved []Decision
	for now := restartAt.Add(time.Second); now.Before(
		restartAt.Add(2 * time.Minute)); now = now.Add(10 * time.Second) {
		for _, d := range r.tick(now) {
			if d.Action == Resolve {
				resolved = append(resolved, d)
			}
		}
	}

	require.Len(t, resolved, 1, "well inside the restore grace")
	assert.Equal(t, 4*time.Minute, resolved[0].Incident.FailedFor(),
		"down until the node came back, not until the grace ended")
}

// A node that is still not ready is not resolved: its findings come back.
func TestRestoredNodeStillNotReadyWaitsForTheGrace(t *testing.T) {
	r, restartAt := restoredNodePage(t, 4*time.Minute, false)

	for now := restartAt.Add(time.Second); now.Before(
		restartAt.Add(9 * time.Minute)); now = now.Add(10 * time.Second) {
		wantNone(t, r.tick(now))
	}
	assert.Equal(t, Open, r.only().State)
}

// A Ready node says nothing about its pressure or other conditions: the
// Ready condition is the recovery of NodeNotReady alone. A restored
// pressure incident keeps the restore grace, so its findings can come back
// before it resolves, reopens and counts a flap for a pressure that never
// stopped.
func TestRestoredNodePressureWaitsForTheGraceWhenTheNodeIsReady(
	t *testing.T,
) {
	pressure := sig(nodeID(), reasons.DiskPressure, detection.Critical)
	r, restartAt := restoredNodeIncident(t, pressure, 4*time.Minute, true)

	for now := restartAt.Add(time.Second); now.Before(
		restartAt.Add(9 * time.Minute)); now = now.Add(10 * time.Second) {
		wantNone(t, r.tick(now))
	}
	assert.Equal(t, Open, r.only().State)
}
