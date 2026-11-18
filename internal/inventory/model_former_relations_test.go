package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormerRelatedKeepsWhatADeletedEntityWasJoinedTo(t *testing.T) {
	m := NewModel(Options{})
	pod := CoreID("pod", "shop", "web-0")
	node := CoreID("node", "", "n1")
	rs := CoreID("replicaset", "shop", "web-1")
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s", At: now,
		Entity: pod})
	_, _ = m.Apply(Observation{Kind: Related, Source: "k8s", At: now,
		Entity: pod, Relation: RunsOn, Targets: []EntityID{node}})
	_, _ = m.Apply(Observation{Kind: Related, Source: "k8s", At: now,
		Entity: pod, Relation: OwnedBy, Targets: []EntityID{rs}})
	assert.Empty(t, m.FormerRelated(pod, RunsOn),
		"an entity never deleted has no former relations")

	_, _ = m.Apply(Observation{Kind: Gone, Source: "k8s",
		At: now.Add(time.Minute), Entity: pod})
	assert.Equal(t, []EntityID{node}, m.FormerRelated(pod, RunsOn))
	assert.Equal(t, []EntityID{rs}, m.FormerRelated(pod, OwnedBy))
	assert.Empty(t, m.Related(pod, RunsOn, Outgoing),
		"the relation leaves the graph with the deletion")

	_, _ = m.Apply(Observation{Kind: Observed, Source: "k8s",
		At: now.Add(2 * time.Minute), Entity: pod})
	assert.Equal(t, []EntityID{node}, m.FormerRelated(pod, RunsOn),
		"a pod of the same name created again keeps where the old ran")
}

func TestFormerRelatedOfUnknownEntityIsEmpty(t *testing.T) {
	m := NewModel(Options{})
	assert.Empty(t, m.FormerRelated(CoreID("pod", "shop", "x"), RunsOn))
}
