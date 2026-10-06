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

// restoredServiceRig announces a page rooted at a Service, restarts, and
// returns the fresh manager's rig with the restore grace ending at
// graceEnd. The Service finding is the one a live session raises again.
func restoredServiceRig(
	t *testing.T, graceEnd time.Duration,
) (*rig, inventory.EntityID, detection.Finding) {
	t.Helper()
	svc := entity(kube.KindService, "warehouse")
	noEndpoints := sig(svc, reasons.NodeNotReady, detection.Critical)
	old := newRig(t, Config{})
	old.raise(at(0), noEndpoints)
	require.Len(t, tickEvery(old, 0, DefaultSettle), 1)
	records := old.m.Export()

	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(graceEnd))
	return fresh, svc, noEndpoints
}

// A restored incident whose Service failure is now explained by a
// crashing Deployment is closed as superseded by the Deployment's
// incident, never as "stable" or "healthy", and chat hears it too.
func TestRestoredIncidentMovesToTheIncidentThatExplainsIt(t *testing.T) {
	grace := 10 * time.Minute
	r, svc, noEndpoints := restoredServiceRig(t, grace)
	oldID := r.only().ID
	deploy := entity(kube.KindDeployment, "warehouse")
	r.cause(svc, deploy, "warehouse crash loops")
	r.raise(at(time.Minute), noEndpoints)

	var got []Decision
	for now := time.Minute; now <= grace+time.Minute; now += 10 * time.Second {
		got = append(got, r.tick(at(now))...)
	}
	var resolve *Decision
	for i, d := range got {
		if d.Action == Resolve {
			resolve = &got[i]
		}
	}
	require.NotNil(t, resolve, "the restored incident is closed: %+v", got)
	assert.Equal(t, oldID, resolve.Incident.ID)
	assert.Equal(t, ReasonSuperseded, resolve.Reason)
	assert.Equal(t, r.idOf(deploy), resolve.Incident.SupersededBy)
	assert.Equal(t, deploy, resolve.Incident.SupersededRoot)
	assert.True(t, resolve.Handover, "chat hears it, not the pagers alone")
	assert.True(t, r.m.holds(r.idOf(deploy), deploy))
}

// Inside the restore grace nothing is handed over: findings are still
// coming back, and the old incident may take them itself.
func TestRestoredIncidentWaitsForTheGraceBeforeHandover(t *testing.T) {
	r, svc, noEndpoints := restoredServiceRig(t, time.Hour)
	r.cause(svc, entity(kube.KindDeployment, "warehouse"), "crash loops")
	r.raise(at(time.Minute), noEndpoints)
	for now := time.Minute; now <= 30*time.Minute; now += 10 * time.Second {
		for _, d := range r.tick(at(now)) {
			assert.NotEqual(t, Resolve, d.Action)
		}
	}
}

// A restored Service incident with no live member anywhere does not end
// as healthy while the Deployment behind it is short and crash looping.
func TestRestoredIncidentOfABrokenServiceNeverResolvesHealthy(t *testing.T) {
	grace := 10 * time.Minute
	r, svc, _ := restoredServiceRig(t, grace)
	deploy, pod := r.workloadRig(t, 2, 0, 5)
	slice := entity(kube.KindEndpointSlice, "warehouse-x")
	r.relate(slice, inventory.Backs, svc)
	r.relate(slice, inventory.RoutesTo, pod)
	_ = deploy
	r.raise(at(time.Second)) // the manager reads the model of an Apply

	for now := time.Duration(0); now <= 3*time.Hour; now += 10 * time.Second {
		for _, d := range r.tick(at(now)) {
			if d.Action == Resolve {
				assert.NotContains(t, string(d.Reason), "healthy")
				assert.NotContains(t, string(d.Reason), "stable")
				return
			}
		}
	}
	assert.Equal(t, Recovering, r.only().State)
}
