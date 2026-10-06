package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var webTerm = kube.PodTerm{
	Selector: []kube.Requirement{{Key: "app", Op: "In",
		Values: []string{"web"}}},
	TopologyKey: "kubernetes.io/hostname",
}

func TestFitAntiAffinityBlocksNodeWithMatchingPod(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, nil)
	fitNodeIn(m, "n2", "b", "z", 4000, nil)
	fitRunning(m, "w1", "n1", "app=web", 100)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{
		Anti: []kube.PodTerm{webTerm}})
	ev := fitOf(t, m, pod)
	assert.Empty(t, ev)
	fitRunning(m, "w2", "n2", "app=web", 100)
	got := fitLines(fitOf(t, m, pod), detection.EvidenceFit)
	assert.Equal(t, []string{
		"pool a: the closest node, n1, already runs d/w1 in its " +
			"kubernetes.io/hostname, which the pod's anti-affinity refuses",
		"pool b: the closest node, n2, already runs d/w2 in its " +
			"kubernetes.io/hostname, which the pod's anti-affinity refuses",
	}, got)
}

func TestFitAffinityNeedsMatchingPodInDomain(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z1", 4000, nil)
	fitNodeIn(m, "n2", "b", "z2", 4000, nil)
	fitRunning(m, "db", "n2", "app=web", 100)
	term := webTerm
	term.TopologyKey = "topology.kubernetes.io/zone"
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{
		Affinity: []kube.PodTerm{term}})
	assert.Empty(t, fitOf(t, m, pod))
}

func TestFitAffinityFirstPodOfGroupMayStart(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z1", 4000, nil)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{
		Affinity: []kube.PodTerm{webTerm}})
	// The pod's own labels (app=web) match the term and nothing else
	// does, so the scheduler lets it start the group.
	assert.Empty(t, fitOf(t, m, pod))
}

func TestFitSpreadSkew(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z1", 4000, nil)
	fitNodeIn(m, "n2", "b", "z2", 4000, nil)
	fitRunning(m, "w1", "n1", "app=web", 100)
	fitRunning(m, "w2", "n1", "app=web", 100)
	spread := kube.Spread{TopologyKey: "topology.kubernetes.io/zone",
		MaxSkew: 1, Selector: webTerm.Selector}
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{
		Spread: []kube.Spread{spread}})
	// Only the empty zone keeps the skew within 1.
	assert.Empty(t, fitOf(t, m, pod))
	fitNodeIn(m, "n2", "b", "z2", 100, nil)
	got := fitLines(fitOf(t, m, pod), detection.EvidenceFit)
	assert.Equal(t, []string{
		"pool a: the closest node, n1, already holds 2 matching pods in " +
			"topology.kubernetes.io/zone z1 while the emptiest has 0 " +
			"(max skew 1)",
		"pool b has no node with 1 CPU free (best: n2 has 100m CPU free)",
	}, got)
}

func TestFitSpreadMinDomainsCountsMissingDomainsAsEmpty(t *testing.T) {
	counts := map[string]int{"z1": 2}
	assert.Equal(t, 0, lowestCount(counts, 3))
	assert.Equal(t, 2, lowestCount(counts, 1))
}

func TestFitPodSelectorOutsideNamespaceIgnored(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "a", "z", 4000, nil)
	other := inventory.EntityID{Kind: kube.KindPod, Namespace: "other",
		Name: "w"}
	observeEntity(m, other, podNodeNow, map[string]inventory.Value{
		kube.AttrPhase:  inventory.Text("Running"),
		kube.AttrLabels: inventory.Text("app=web")})
	relateEntity(m, other, inventory.RunsOn,
		inventory.EntityID{Kind: kube.KindNode, Name: "n1"})
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{
		Anti: []kube.PodTerm{webTerm}})
	assert.Empty(t, fitOf(t, m, pod))
}
