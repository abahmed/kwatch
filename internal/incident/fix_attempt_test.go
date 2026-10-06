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

// openedRollout announces an incident rooted at a Deployment.
func openedRollout(t *testing.T) (*rig, inventory.EntityID, detection.Finding) {
	r := newRig(t, Config{})
	web := entity(kube.KindDeployment, "web")
	r.observe(web)
	failing := sig(web, reasons.ProgressDeadlineExceeded, detection.Warning)
	r.raise(at(0), failing)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	return r, web, failing
}

// touch tells the manager that id changed, as the engine does.
func (r *rig) touch(now time.Time, id inventory.EntityID) {
	r.m.Apply(r.snapshot(now), []inventory.EntityID{id}, nil)
}

func rollout(revision string) inventory.Change {
	change := image("web:v1", "web:v2")
	change.Revision, change.Actor = revision, "alice"
	return change
}

// A rollout started while the incident is open is one thread update;
// scaling is none; and if the incident still fails 10 minutes later the
// thread says so once.
func TestFixAttemptIsReportedOnceAndWatched(t *testing.T) {
	r, web, _ := openedRollout(t)

	r.change(web, 2*time.Minute, inventory.Change{Fields: []inventory.FieldChange{
		{Path: "spec.replicas", Before: "2", After: "4"}}})
	r.touch(at(2*time.Minute), web)
	wantNone(t, r.tick(at(2*time.Minute)))

	r.change(web, 3*time.Minute, rollout("15"))
	r.touch(at(3*time.Minute), web)
	ds := r.tick(at(3 * time.Minute))
	wantAction(t, ds, Update, ReasonFixAttempt)
	require.NotNil(t, ds[0].Incident.Attempt)
	assert.Equal(t, "15", ds[0].Incident.Attempt.Revision)
	wantNone(t, r.tick(at(4*time.Minute)))

	r.touch(at(5*time.Minute), web)
	wantNone(t, r.tick(at(5*time.Minute)))

	wantAction(t, r.tick(at(13*time.Minute)), Update,
		ReasonFixStillFailing)
	wantNone(t, r.tick(at(20*time.Minute)))
}

// A second change is a second attempt, and a change made before the
// announcement is the cause's business, not an attempt.
func TestFixAttemptIgnoresChangesBeforeTheAnnouncement(t *testing.T) {
	r := newRig(t, Config{})
	web := entity(kube.KindDeployment, "web")
	r.observe(web)
	r.change(web, -time.Minute, rollout("14"))
	r.raise(at(0), sig(web, reasons.ProgressDeadlineExceeded,
		detection.Warning))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.touch(at(DefaultSettle), web)
	wantNone(t, r.tick(at(DefaultSettle+time.Second)))

	r.change(web, 3*time.Minute, rollout("15"))
	r.touch(at(3*time.Minute), web)
	wantAction(t, r.tick(at(3*time.Minute)), Update, ReasonFixAttempt)
	r.change(web, 4*time.Minute, rollout("16"))
	r.touch(at(4*time.Minute), web)
	ds := r.tick(at(4 * time.Minute))
	wantAction(t, ds, Update, ReasonFixAttempt)
	assert.Equal(t, "16", ds[0].Incident.Attempt.Revision)
}

// The resolve keeps the attempt, so it can say what fixed the incident.
func TestResolveAfterFixAttemptNamesTheChange(t *testing.T) {
	r, web, failing := openedRollout(t)
	r.change(web, 3*time.Minute, rollout("15"))
	r.touch(at(3*time.Minute), web)
	wantAction(t, r.tick(at(3*time.Minute)), Update, ReasonFixAttempt)

	r.clear(at(8*time.Minute), failing)
	var got Decision
	for now := 8 * time.Minute; now < time.Hour; now += 10 * time.Second {
		if ds := r.tick(at(now)); len(ds) == 1 {
			got = ds[0]
			break
		}
	}
	require.Equal(t, Resolve, got.Action)
	require.NotNil(t, got.Incident.Attempt)
	require.NotNil(t, got.Incident.FixedBy)
	assert.Equal(t, "15", got.Incident.FixedBy.Revision)
}

// A fix attempt seen while a held material change ends the tick is not
// lost: it is told on a later tick, once the material update is out.
func TestFixAttemptSeenWhileMaterialChangeIsHeldIsToldLater(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.raise(at(2*time.Minute), crashSig(pod))
	wantAction(t, r.tick(at(2*time.Minute)), Update, ReasonMaterialChange)

	r.raise(at(4*time.Minute), sig(deploy,
		reasons.DeploymentUnavailable, detection.Warning))
	wantNone(t, r.tick(at(4*time.Minute)))
	r.change(deploy, 5*time.Minute, rollout("15"))
	r.touch(at(5*time.Minute), deploy)
	wantNone(t, r.tick(at(5*time.Minute)))

	held := 2*time.Minute + MaterialGap
	wantAction(t, r.tick(at(held)), Update, ReasonMaterialChange)
	ds := r.tick(at(held + time.Second))
	wantAction(t, ds, Update, ReasonFixAttempt)
	assert.Equal(t, "15", ds[0].Incident.Attempt.Revision)
}

// The same holds for a cause revision that is still settling.
func TestFixAttemptSeenWhileRevisionSettlesIsToldLater(t *testing.T) {
	r, web, _ := openedRollout(t)
	p := r.only()
	r.m.mu.Lock()
	r.m.incidents[p.ID].Pending.MarkRevised(at(2 * time.Minute))
	r.m.mu.Unlock()
	r.change(web, 2*time.Minute, rollout("15"))
	r.touch(at(2*time.Minute), web)
	wantNone(t, r.tick(at(2*time.Minute)))

	wantAction(t, r.tick(at(2*time.Minute+DefaultReviseSettle)), Update,
		ReasonCauseRevised)
	wantAction(t, r.tick(at(3*time.Minute)), Update, ReasonFixAttempt)
}
