package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// totalProbe records the node total the pass sample saw.
type totalProbe struct{ total *int }

func (totalProbe) Name() string { return "total-probe" }
func (totalProbe) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}
func (p totalProbe) Detect(
	ctx detection.Context, _ inventory.Entity,
) []detection.Finding {
	*p.total = fitSampleFor(ctx).total
	return nil
}

// A node added after the first evaluation at the same instant is seen
// by the second one, not hidden behind the cached sample.
func TestFitSampleSeesNodesAddedAtTheSameInstant(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	observe := func(id inventory.EntityID) {
		_, _ = model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", At: now, Entity: id})
	}
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "ns", Name: "p"}
	observe(pod)
	observe(inventory.EntityID{Kind: kube.KindNode, Name: "n1"})
	total := 0
	r := detection.NewRegistry(nil, totalProbe{total: &total})

	r.Evaluate(model, now, pod)
	assert.Equal(t, 1, total)
	observe(inventory.EntityID{Kind: kube.KindNode, Name: "n2"})
	r.Evaluate(model, now, pod)
	assert.Equal(t, 2, total)
}
