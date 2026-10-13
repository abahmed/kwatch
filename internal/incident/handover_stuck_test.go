package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

func stuckSig(name string) detection.Finding {
	f := sig(entity(kube.KindSecret, name), "StuckDeleting",
		detection.Warning)
	f.Mode = detection.ModeStuckDeleting
	return f
}

// Objects held by one finalizer were once blamed on a stopped workload.
// After a restart they are blamed on the finalizer: the restored incident
// of the workload has no member left, and its problem lives on in the
// finalizer's incident, so it is handed over, not resolved as healthy.
func TestRestoredStuckDeletionHandsOverToTheFinalizerIncident(t *testing.T) {
	grace := 10 * time.Minute
	a := stuckSig("old-a")
	ctl := entity(kube.KindDeployment, "legacy")
	old := newRig(t, Config{})
	old.cause(a.Entity, ctl, "controller scaled to zero")
	announced(t, old, a)
	oldID := old.of(ctl).ID

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(grace))
	fin := inventory.CoreID(explain.KindFinalizer, "shop", "x.io/cleanup")
	fresh.cause(a.Entity, fin, "finalizer is never removed")
	fresh.raise(at(time.Minute), a)

	var resolve *Decision
	for now := time.Minute; now <= grace+time.Minute; now += 10 * time.Second {
		got := fresh.tick(at(now))
		for i, d := range got {
			if d.Action == Resolve && d.Incident.ID == oldID {
				resolve = &got[i]
			}
		}
	}
	require.NotNil(t, resolve, "the restored incident is closed")
	assert.Equal(t, ReasonSuperseded, resolve.Reason)
	assert.Equal(t, fresh.idOf(fin), resolve.Incident.SupersededBy)
	assert.Empty(t, fresh.m.RestoredFailing(), "one problem, one incident")
}
