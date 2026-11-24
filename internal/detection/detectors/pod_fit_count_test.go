package detectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// listCounter counts the full listings of each kind read through it.
type listCounter struct {
	*inventory.Model
	lists map[inventory.Kind]int
}

func (l *listCounter) Entities(kind inventory.Kind) []inventory.EntityID {
	l.lists[kind]++
	return l.Model.Entities(kind)
}

// Every unschedulable pod of a pass reads the node sample, but the pass
// lists the nodes once and never lists all pods just to key the sample:
// with thousands of pods that copy and sort under the model's lock
// would be paid per pod.
func TestFitSampleDoesNotListAllPodsPerPod(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	model := &listCounter{Model: inventory.NewModel(inventory.Options{}),
		lists: map[inventory.Kind]int{}}
	observe := func(id inventory.EntityID) {
		_, _ = model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", At: now, Entity: id})
	}
	observe(inventory.EntityID{Kind: kube.KindNode, Name: "n1"})
	const pods = 200
	for i := range pods {
		observe(inventory.EntityID{Kind: kube.KindPod, Namespace: "ns",
			Name: fmt.Sprintf("p-%d", i)})
	}
	total := 0
	r := detection.NewRegistry(nil, totalProbe{total: &total})

	for i := range pods {
		r.Evaluate(model, now, inventory.EntityID{Kind: kube.KindPod,
			Namespace: "ns", Name: fmt.Sprintf("p-%d", i)})
	}

	assert.Equal(t, 1, total)
	assert.Zero(t, model.lists[kube.KindPod], "pods are counted, not listed")
	assert.Equal(t, 1, model.lists[kube.KindNode], "nodes listed once")
}
