package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// backoffRig announces a crash loop of a Deployment that reports all
// its replicas ready, as it does between two crashes, then clears the
// finding the way a quiet back-off does.
func backoffRig(t *testing.T) (*rig, inventory.EntityID) {
	t.Helper()
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 1, 1, 6)
	container := entity(kube.KindContainer, "api-1.app")
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	crash := crashSig(pod)
	r.raise(at(0), crash)
	require.Len(t, r.tick(at(DefaultSettle)), 1)
	r.clear(at(5*time.Minute), crash)
	r.tick(at(5 * time.Minute))
	return r, container
}

// A container waiting in back-off is not healthy: the hold is shorter
// than the back-off, so the quiet between two crashes must not read as
// recovery.
func TestCrashBackoffNeverCountsAsHealthy(t *testing.T) {
	r, container := backoffRig(t)
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff)})

	for now := 6 * time.Minute; now < 40*time.Minute; now += 4 * time.Minute {
		wantNone(t, r.tick(at(now)))
	}
	assert.Equal(t, Recovering, r.only().State)

	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrStateReason: inventory.Text("")})
	ds := r.tick(at(41 * time.Minute))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}

// A container that crashed within the hold, and runs for the moment,
// is not healthy either.
func TestRecentCrashDoesNotCountAsHealthy(t *testing.T) {
	r, container := backoffRig(t)
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrLastFinished: inventory.Time(at(5*time.Minute + 30*time.Second)),
		kube.AttrLastExitCode: inventory.Number(1),
	})
	wantNone(t, r.tick(at(5*time.Minute+DefaultHold)))
	wantNone(t, r.tick(at(5*time.Minute+30*time.Second+DefaultHold-
		time.Second)))
	ds := r.tick(at(5*time.Minute + 30*time.Second + DefaultHold +
		time.Second))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}

// A run that ended cleanly, such as an init container's, is no crash.
func TestCleanExitDoesNotHoldTheResolve(t *testing.T) {
	r, container := backoffRig(t)
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrLastFinished: inventory.Time(
			at(5*time.Minute + 30*time.Second)),
		kube.AttrLastExitCode: inventory.Number(0),
	})
	ds := r.tick(at(5*time.Minute + DefaultHold + time.Second))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}

// nodeTimelineRig is a node incident whose timeline names the api
// Deployment, whose one pod waits in back-off. onNode puts that pod on
// the node.
func nodeTimelineRig(t *testing.T, onNode bool) (*rig, *Incident) {
	t.Helper()
	r := newRig(t, Config{})
	r.m.model = r.model
	deploy, pod := r.workloadRig(t, 1, 1, 6)
	r.setAttrs(entity(kube.KindContainer, "api-1.app"),
		map[string]inventory.Value{kube.AttrStateReason: inventory.Text(
			reasons.CrashLoopBackOff)})
	node := inventory.CoreID(kube.KindNode, "", "n1")
	r.setAttrs(node, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	if onNode {
		r.relate(pod, inventory.RunsOn, node)
	}
	return r, &Incident{Root: node, Timeline: []Event{
		{At: at(0), Text: "joined", Entity: &deploy}}}
}

// A node incident is held only by a crash loop on that node: a workload
// its timeline names that crashes elsewhere is not the node's failure.
func TestNodeIncidentIgnoresCrashLoopsOffTheNode(t *testing.T) {
	r, p := nodeTimelineRig(t, false)
	assert.False(t, r.m.crashing(p, at(time.Hour)))

	r, p = nodeTimelineRig(t, true)
	assert.True(t, r.m.crashing(p, at(time.Hour)))
}

// A root with pods of its own is judged by them, not by the other
// workloads its timeline names.
func TestOwnPodsDecideOverTimelineWorkloads(t *testing.T) {
	r, p := nodeTimelineRig(t, true)
	web, webPod := entity(kube.KindDeployment, "web"),
		entity(kube.KindPod, "web-1")
	r.relate(webPod, inventory.OwnedBy, web)
	p.Root = web
	assert.False(t, r.m.crashing(p, at(time.Hour)))
}

// At the cap, a workload at its replicas held only by a crash loop is
// said to be crashing, not short of replicas.
func TestCappedResolveSaysWhichConditionHeld(t *testing.T) {
	r, container := backoffRig(t)
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff)})
	ds := tickEvery(r, 6*time.Minute, 5*time.Minute+StillBrokenMax)
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
	assert.Equal(t, Reason(StoppedTrackingPrefix+
		"2h; coverage check continues"+StillCrashingNote), ds[0].Reason)
}

// A crash loop whose container looks ready between crashes (it has no
// readiness probe) comes back again and again; each return is the next
// crash, not a recovery that failed again. It stays open and never
// flaps.
func TestCrashLoopGapsAreNoFlapCycles(t *testing.T) {
	r, container := backoffRig(t)
	pod := entity(kube.KindPod, "api-1")
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff)})
	now := 5 * time.Minute
	for i := 0; i < 2*DefaultFlapCycles; i++ {
		now += time.Minute
		r.raise(at(now), crashSig(pod))
		r.tick(at(now))
		now += time.Minute
		r.clear(at(now), crashSig(pod))
		r.tick(at(now))
	}
	r.raise(at(now+time.Minute), crashSig(pod))
	r.tick(at(now + time.Minute))
	got := r.only()
	assert.NotEqual(t, Flapping, got.State)
	// The rig's first recovery came before the container reported its
	// back-off; every later one was a gap of the crash loop.
	assert.LessOrEqual(t, len(got.Cycles), 1)
}

// A crash-loop gap noted while an incident recovered is dropped when it
// resolves, so a later recovery is not taken for a gap.
func TestCrashGapForgottenOnResolve(t *testing.T) {
	r := newRig(t, Config{})
	p := &Incident{ID: "inc-gap", State: Recovering}
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	r.m.gaps = map[string]bool{p.ID: true}
	r.m.markResolved(p)
	if r.m.gaps[p.ID] {
		t.Fatal("the gap outlived the incident's resolve")
	}
}
