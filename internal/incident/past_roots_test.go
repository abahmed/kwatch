package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// movedRig announces a crash blamed on a ConfigMap, revises the cause to
// the Deployment, keeps the incident open for openFor and lets it
// resolve. It returns the rig, the crash, the ConfigMap, the incident ID
// and when it resolved.
func movedRig(t *testing.T, openFor time.Duration) (
	*rig, detection.Finding, inventory.EntityID, string, time.Time,
) {
	t.Helper()
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	crash := sig(pod, "CrashLoopBackOff", detection.Warning)
	config := cm("web-config")
	r.cause(pod, config, "configmap web-config changed")
	announced(t, r, crash)
	id := r.idOf(config)

	r.cause(pod, deploy, "deployment web changed")
	r.apply(at(2*time.Minute), detection.Changed, crash)
	r.tick(at(2*time.Minute + DefaultReviseSettle))
	require.Equal(t, deploy, r.only().Root, "the cause moved")

	r.clear(at(openFor), crash)
	r.tick(at(openFor))
	resolved := openFor + DefaultHold
	ds := r.tick(at(resolved))
	require.Len(t, ds, 1)
	require.Equal(t, Resolve, ds[0].Action)
	r.cause(pod, config, "configmap web-config changed")
	return r, crash, config, id, at(resolved)
}

// The original problem returns within RepageWindow of the resolve, but
// long after the cause moved: it reopens the incident whatever its root
// was revised to, and says it is failing again.
func TestReturnAtFormerRootReopensAfterLongMove(t *testing.T) {
	r, crash, config, id, resolved := movedRig(t, 3*time.Hour)

	back := resolved.Add(20 * time.Minute)
	r.raise(back, crash)
	r.tick(back)
	ds := r.tick(back.Add(DefaultReviseSettle))
	require.Len(t, r.m.Export(), 1, "no second incident")
	wantAction(t, ds, Update, ReasonFailingAgain)
	got := ds[0].Incident
	assert.Equal(t, id, got.ID)
	assert.Equal(t, config, got.Root, "back where the reasoning puts it")
	assert.Zero(t, r.m.resolved.count, "no longer filed as resolved")
	p := r.m.incidents[id]
	assert.Contains(t, p.pastRoots, entity(kube.KindDeployment, "web"))
}

// After RepageWindow the return is news, an incident of its own at the
// root the reasoning names now, linked to the earlier one.
func TestReturnAtFormerRootLinksPreviousAfterWindow(t *testing.T) {
	r, crash, config, id, resolved := movedRig(t, 3*time.Hour)

	back := resolved.Add(RepageWindow + time.Hour)
	r.raise(back, crash)
	r.tick(back)
	got := r.of(config)
	assert.NotEqual(t, id, got.ID)
	assert.Equal(t, id, got.Previous, "the history is kept")
	assert.Len(t, got.History, 1)
}

// A short move is found the same way as before: within the window the
// return reopens the incident.
func TestReturnAtFormerRootReopensAfterShortMove(t *testing.T) {
	r, crash, _, id, resolved := movedRig(t, 10*time.Minute)

	back := resolved.Add(20 * time.Minute)
	r.raise(back, crash)
	r.tick(back)
	ds := r.tick(back.Add(DefaultReviseSettle))
	wantAction(t, ds, Update, ReasonFailingAgain)
	assert.Equal(t, id, ds[0].Incident.ID)
}

// The roots an incident had survive a restart.
func TestPastRootsSurviveRestore(t *testing.T) {
	r, crash, config, id, resolved := movedRig(t, 3*time.Hour)
	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), time.Time{})
	pod, _ := workload(fresh, "web")
	require.Equal(t, crash.Entity, pod)
	fresh.cause(pod, config, "configmap web-config changed")

	back := resolved.Add(RepageWindow + time.Hour)
	fresh.raise(back, crash)
	fresh.tick(back)
	assert.Equal(t, id, fresh.of(config).Previous)
}

// A recurrence carries the roots of the incident it replaces, so a
// later return at either root still finds the history.
func TestRecurrenceCarriesPastRoots(t *testing.T) {
	p := &Incident{Root: cm("a")}
	previous := &Incident{Root: entity(kube.KindDeployment, "web"),
		pastRoots: []inventory.EntityID{cm("a"), cm("b")}}
	carryRoots(p, previous)
	assert.ElementsMatch(t, []inventory.EntityID{
		entity(kube.KindDeployment, "web"), cm("b")}, p.pastRoots)
}
