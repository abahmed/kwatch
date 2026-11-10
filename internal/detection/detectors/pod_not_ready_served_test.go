package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// unreadyWithBudget is a running pod whose container declares a ten
// minute startup budget, unready for five minutes. servedFirst says it
// had been ready after its containers started.
func unreadyWithBudget(servedFirst bool) int {
	started := podNodeNow.Add(-time.Hour)
	readyFalse := podNodeNow.Add(-5 * time.Minute)
	if !servedFirst {
		started = readyFalse
	}
	model := newTestModel()
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "app"}
	observeEntity(model, pod, started, map[string]inventory.Value{
		kube.AttrPhase:             inventory.Text("Running"),
		kube.AttrReady:             inventory.Bool(false),
		kube.AttrReadySince:        inventory.Time(readyFalse),
		kube.AttrContainersStarted: inventory.Time(started),
	})
	id := kube.ContainerID("default", "app", "main")
	observeEntity(model, id, started, map[string]inventory.Value{
		kube.AttrProbeBudget: inventory.Number(600),
	})
	relateEntity(model, id, inventory.PartOf, pod)
	return len(NewPod(PodThresholds{NotReady: 3 * time.Minute}).Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, pod)))
}

// The probe budget is for a pod still starting. A pod that served and
// then lost readiness is judged by the NotReady threshold alone.
func TestNotReadyBudgetOnlyForPodsNeverServing(t *testing.T) {
	assert.Equal(t, 1, unreadyWithBudget(true))
	assert.Equal(t, 0, unreadyWithBudget(false))
}
