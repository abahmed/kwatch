package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A cause revision moves an incident onto a root whose own incident is
// still settling, and merges that one in. The merged incident is gone,
// so neither a node-outage group nor a crash gap may keep its ID.
func TestRerootMergeLeavesNoIndexOnTheMergedIncident(t *testing.T) {
	r := newRig(t, Config{})
	m := r.m
	root := entity(kube.KindDeployment, "api")
	node := entity(kube.KindNode, "n1")
	target := &Incident{ID: "settling", Root: root, State: Settling,
		Merged: []inventory.EntityID{node},
		Members: map[detection.Key]detection.Finding{
			podSig("api-1").Key(): podSig("api-1")}}
	moved := &Incident{ID: "moved", Root: entity(kube.KindPod, "web"),
		State: Open, Members: map[detection.Key]detection.Finding{}}
	for _, p := range []*Incident{target, moved} {
		m.incidents[p.ID] = p
		m.index(p)
	}
	m.groups[node.String()] = target.ID
	if m.gaps == nil {
		m.gaps = map[string]bool{}
	}
	m.gaps[target.ID] = true

	m.reroot(at(0), moved, root)

	assert.NotContains(t, m.incidents, target.ID)
	assert.NotContains(t, m.groups, node.String())
	assert.NotContains(t, m.gaps, target.ID)
	assert.Equal(t, moved, m.lookup(root))
}
