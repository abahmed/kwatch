package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A completed init run is a sample of the pod's workload, once.
func TestBaselineSamplerInitRunOnce(t *testing.T) {
	r := newSamplerRig(t)
	container := kube.ContainerID("shop", "api-1-a", "wait-for-db")
	r.apply(inventory.Observation{Kind: inventory.Related,
		Entity: container, Relation: inventory.PartOf,
		Targets: []inventory.EntityID{r.pod}})
	attrs := map[string]inventory.Value{
		kube.AttrInitSeconds: inventory.Number(42)}
	r.observe(container, attrs)
	r.observe(container, attrs)

	stat := r.stat(inventory.MetricInitSeconds)
	assert.Equal(t, 1, stat.Count)
	assert.InDelta(t, 42, stat.Mean, 0.5)
}
