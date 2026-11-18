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

// youngPod puts pod, owned by replica set api-1, on node, which joined
// at joined; a failing pod is not ready and logs a refused connection.
func (r *rig) youngPod(pod, node string, joined time.Time, failing bool) {
	r.t.Helper()
	n := inventory.CoreID(kube.KindNode, "", node)
	p := inventory.CoreID(kube.KindPod, "shop", pod)
	c := kube.ContainerID("shop", pod, "app")
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-1")
	r.setAttrs(rs, map[string]inventory.Value{})
	r.setAttrs(n, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(joined)})
	r.setAttrs(p, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(!failing)})
	r.setAttrs(c, map[string]inventory.Value{
		kube.AttrLastErrorLine: inventory.Text(refusedLine)})
	r.relate(c, inventory.PartOf, p)
	r.relate(p, inventory.RunsOn, n)
	r.relate(p, inventory.OwnedBy, rs)
}

func youngSig(pod string, since time.Time) detection.Finding {
	return detection.Finding{Reason: "CrashLoop",
		Entity:   kube.ContainerID("shop", pod, "app"),
		Severity: detection.Warning, Since: since,
		Summary: "CrashLoop on " + pod}
}

func youngRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, Config{})
	r.m.model = r.model
	return r
}

// The hold ends kube.YoungNodeWindow after the node joined, not after
// the longer kube.BootWindow.
func TestYoungHoldEndsAfterTheYoungNodeWindow(t *testing.T) {
	r := youngRig(t)
	r.youngPod("api-1-a", "old", t0.Add(-24*time.Hour), false)
	r.youngPod("api-1-b", "new", t0.Add(-2*time.Minute), true)
	r.raise(at(0), youngSig("api-1-b", at(0)))
	require.Empty(t, r.m.Export(), "held while the node is new")

	r.tick(at(time.Minute - time.Second))
	require.Empty(t, r.m.Export())
	r.tick(at(time.Minute))
	assert.Len(t, r.m.Export(), 1,
		"reported once the node is kube.YoungNodeWindow old")
}

// A cold start: every node is new, so no older node can show that the
// failure is the node's. The short window is then the only bound.
func TestYoungHoldOnColdStartReportsAfterThreeMinutes(t *testing.T) {
	r := youngRig(t)
	r.youngPod("api-1-a", "n1", t0, true)
	r.youngPod("api-1-b", "n2", t0, true)
	r.raise(at(30*time.Second), youngSig("api-1-a", at(30*time.Second)),
		youngSig("api-1-b", at(30*time.Second)))
	require.Empty(t, r.m.Export(), "held while every node is new")

	r.tick(at(kube.YoungNodeWindow - time.Second))
	require.Empty(t, r.m.Export())
	r.tick(at(kube.YoungNodeWindow))
	assert.NotEmpty(t, r.m.Export(),
		"a failure that persists is reported at the window's end")
}

// A failure of the same cause on a node older than the window opens the
// incident at once, and releases what was held for that cause.
func TestYoungHoldReleasedByAFailureOnAnOldNode(t *testing.T) {
	r := youngRig(t)
	r.youngPod("api-1-a", "old", t0.Add(-24*time.Hour), false)
	r.youngPod("api-1-b", "new", t0.Add(-time.Minute), true)
	root := inventory.CoreID(kube.KindService, "shop", "redis")
	for _, pod := range []string{"api-1-a", "api-1-b"} {
		r.cause(kube.ContainerID("shop", pod, "app"), root, "redis down")
	}
	r.raise(at(0), youngSig("api-1-b", at(0)))
	require.Empty(t, r.m.Export())

	r.youngPod("api-1-a", "old", t0.Add(-24*time.Hour), true)
	r.raise(at(10*time.Second), youngSig("api-1-a", at(10*time.Second)))
	assert.Len(t, r.of(root).Members, 2,
		"the held failure joined the incident at once")
}

// A pod of the same failure on a newer node never makes the hold longer.
func TestYoungHoldNeverMovesLater(t *testing.T) {
	r := youngRig(t)
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-1")
	r.youngPod("api-1-b", "new", t0.Add(-2*time.Minute), true)
	notReady := sig(rs, "NotReady", detection.Warning)
	r.raise(at(0), notReady)
	require.Empty(t, r.m.Export())

	r.youngPod("api-1-c", "newer", at(30*time.Second), true)
	r.raise(at(30*time.Second), notReady)
	require.Empty(t, r.m.Export())
	r.tick(at(time.Minute))
	assert.Len(t, r.m.Export(), 1,
		"the hold ended when it was first due, not 3 minutes later")
}
