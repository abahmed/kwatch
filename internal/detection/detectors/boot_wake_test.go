package detectors

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// wakeRig is the pod of replacementRig on an hour-old node, while
// wakeApps workloads start from zero replicas, one a minute from t0.
func wakeRig(wakeApps int) (*inventory.Model, inventory.EntityID) {
	model, pod := replacementRig(time.Hour)
	for i := range wakeApps {
		id := newID(kube.KindDeployment, "shop", "app"+strconv.Itoa(i))
		at := t0.Add(time.Duration(i) * time.Minute)
		put(model, id, at, map[string]inventory.Value{
			kube.AttrReplicas: inventory.Number(2)})
		model.Apply(inventory.Observation{
			Kind: inventory.Changed, Source: "test", At: at, Entity: id,
			Change: inventory.Change{Entity: id, At: at,
				Fields: []inventory.FieldChange{{Path: "spec.replicas",
					Before: "0", After: "2"}}},
		})
	}
	return model, pod
}

func TestWakeUpHoldsNotReadyUntilTheWakeUpIsOver(t *testing.T) {
	model, pod := wakeRig(kube.WakeMinWorkloads + 1)
	end := t0.Add(5*time.Minute + kube.WakeQuiet)

	assert.Empty(t, podFindings(model, pod, t0.Add(4*time.Minute)),
		"the usual threshold does not apply while the cluster wakes up")
	assert.Empty(t, podFindings(model, pod, end.Add(-time.Second)))
	got := podFindings(model, pod, end)
	assert.Equal(t, []string{reasons.ContainersNotReady}, got,
		"a pod still not ready when the wake-up ends is reported at once")
}

func TestFewStartsAreNotAWakeUp(t *testing.T) {
	model, pod := wakeRig(kube.WakeMinWorkloads - 1)

	got := podFindings(model, pod, t0.Add(4*time.Minute))

	assert.Equal(t, []string{reasons.ContainersNotReady}, got)
}

func TestWakeUpDoesNotExcuseAPodTheWakeUpDidNotCreate(t *testing.T) {
	model, pod := wakeRig(kube.WakeMinWorkloads + 1)
	put(model, pod, t0, map[string]inventory.Value{
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(t0),
		kube.AttrCreated:    inventory.Time(t0.Add(-time.Hour))})

	got := podFindings(model, pod, t0.Add(4*time.Minute))

	assert.Equal(t, []string{reasons.ContainersNotReady}, got)
}

func TestWakeUpDoesNotExcuseACrashLoop(t *testing.T) {
	model, pod := wakeRig(kube.WakeMinWorkloads + 1)
	container := newID(kube.KindContainer, "shop", "web-1/app")
	put(model, container, t0, map[string]inventory.Value{
		kube.AttrState:       inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff)})
	relateEntity(model, container, inventory.PartOf, pod)
	ctx := testDetectorContext(model, t0.Add(4*time.Minute))

	assert.Zero(t, bootGraceFor(ctx, entityOf(model, pod)))
}
