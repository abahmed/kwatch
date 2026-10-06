package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// bootRig is a not-ready pod, unready since t0, on one of three nodes
// of a pool created at t0.
func bootRig() (*inventory.Model, inventory.EntityID) {
	model, pod := replacementRig(0)
	pool := newID(kube.KindNodePool, "", "workers")
	for _, name := range []string{"n1", "n2"} {
		node := newID(kube.KindNode, "", name)
		put(model, node, t0, map[string]inventory.Value{
			kube.AttrCreated: inventory.Time(t0)})
		relateEntity(model, node, inventory.PartOf, pool)
	}
	relateEntity(model, newID(kube.KindNode, "", "n2"), inventory.PartOf,
		pool)
	return model, pod
}

func TestBootingPoolHoldsBackNotReadyUntilTheBootIsOver(t *testing.T) {
	model, pod := bootRig()

	assert.Empty(t, podFindings(model, pod,
		t0.Add(kube.BootWindow-time.Second)),
		"inside the boot a not-ready pod is expected")
	got := podFindings(model, pod, t0.Add(kube.BootWindow))
	assert.Equal(t, []string{reasons.ContainersNotReady}, got,
		"a pod still not ready when the boot ends is reported at once")
}

func TestBootGraceSkipsAPodThatCrashes(t *testing.T) {
	model, pod := bootRig()
	container := newID(kube.KindContainer, "shop", "web-1/app")
	put(model, container, t0, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff)})
	relateEntity(model, container, inventory.PartOf, pod)
	ctx := testDetectorContext(model, t0.Add(time.Minute))

	assert.Zero(t, bootGraceFor(ctx, entityOf(model, pod)),
		"a crash loop is not a slow start")
}

func TestBootSandboxEventsAreQuietUntilTheBootIsOver(t *testing.T) {
	model, pod := bootRig()
	noteEntity(model, pod, "FailedCreatePodSandBox", "cni not ready", 1, t0)
	detect := func(now time.Time) []string {
		ctx := testDetectorContext(model, now)
		return findingReasons(Event{}.Detect(ctx, entityOf(model, pod)))
	}

	assert.Empty(t, detect(t0.Add(time.Minute)))
	require.Contains(t, detect(t0.Add(kube.BootWindow)),
		"FailedCreatePodSandBox")
}
