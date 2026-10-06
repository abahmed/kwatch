package detectors

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// fitNodeIn adds a Ready node with cpu millicores in a pool and zone;
// attrs add to or replace the defaults.
func fitNodeIn(
	m *inventory.Model, name, pool, zone string, cpu float64,
	attrs map[string]inventory.Value,
) inventory.EntityID {
	merged := map[string]inventory.Value{
		kube.AttrCPUAllocatable:    inventory.Number(cpu),
		kube.AttrMemoryAllocatable: inventory.Number(64 * gib),
		kube.AttrReady:             inventory.Bool(true),
	}
	for k, v := range attrs {
		merged[k] = v
	}
	id := inventory.EntityID{Kind: kube.KindNode, Name: name}
	observeEntity(m, id, podNodeNow, merged)
	var parents []inventory.EntityID
	if pool != "" {
		parents = append(parents, inventory.CoreID(kube.KindNodePool,
			"", pool))
	}
	if zone != "" {
		parents = append(parents, inventory.CoreID(kube.KindZone, "", zone))
	}
	if len(parents) > 0 {
		relateEntity(m, id, inventory.PartOf, parents...)
	}
	return id
}

// fitPending adds a Pending pod asking cpu millicores with the spec.
func fitPending(
	m *inventory.Model, name string, cpu float64, spec kube.SchedulingSpec,
) inventory.EntityID {
	id := capPod(m, name, "", "Pending", cpu, 0)
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	observeEntity(m, id, podNodeNow, map[string]inventory.Value{
		kube.AttrSchedulingSpec: inventory.Text(string(data)),
		kube.AttrLabels:         inventory.Text("app=web"),
	})
	return id
}

// fitOf is the scheduling explanation of a pod.
func fitOf(t *testing.T, m *inventory.Model, pod inventory.EntityID,
) []detection.Evidence {
	t.Helper()
	entity, ok := m.Entity(pod)
	require.True(t, ok)
	return fitEvidence(testDetectorContext(m, podNodeNow), entity)
}

// fitLines returns the values of one evidence label.
func fitLines(ev []detection.Evidence, label string) []string {
	var out []string
	for _, e := range ev {
		if e.Label == label {
			out = append(out, e.Value)
		}
	}
	return out
}

// fitRunning adds a running pod with the labels on a node.
func fitRunning(
	m *inventory.Model, name, node, labels string, cpu float64,
) {
	id := capPod(m, name, node, "Running", cpu, 0)
	observeEntity(m, id, podNodeNow, map[string]inventory.Value{
		kube.AttrLabels: inventory.Text(labels)})
}
