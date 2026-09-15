package insight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

func TestAnalyzeRecentChangesCap(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	now := time.Now()
	// Five edits to the workload itself; the alert shows the three most
	// recent rather than a wall of them.
	for i := 0; i < 5; i++ {
		tracker.Record(context.Change{
			Resource:  "deploy",
			Namespace: "ns1",
			Name:      "dep1",
			Type:      context.ChangeUpdate,
			Timestamp: now.Add(-time.Duration(5-i) * time.Second),
		})
	}

	e := NewEngine(nil, tracker)
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "deploy",
			Namespace: "ns1",
			Name:      "dep1",
		},
	}
	ins := e.Analyze(inc)

	assert.Len(t, ins.RecentChanges, 3)
}

func TestAnalyzeDependencyChangeDoesNotClobberCause(t *testing.T) {
	graph := context.NewResourceGraph()
	// Pod p1 depends on configmap cm1 AND its scheduled node (n1).
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	graph.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")

	tracker := context.NewChangeTracker(100)
	// The configmap was changed recently — an irrelevant-but-present signal.
	tracker.Record(context.Change{
		Resource: "configmap", Namespace: "ns1", Name: "cm1",
		Type: context.ChangeUpdate, Timestamp: time.Now(),
	})

	e := NewEngine(graph, tracker)
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
			NodeName:  "n1",
		},
	}

	ins := e.Analyze(inc)

	// The node is the specific diagnosis; the configmap update must not
	// override it with a generic dependency_change wording.
	assert.Equal(t, "node n1 may be unhealthy", ins.Cause)
	assert.Equal(t, "node_failure", ins.Pattern)
	assert.Len(t, ins.RecentChanges, 1) // still surfaced as context
}

func TestAnalyzeImpactTransitiveThroughService(t *testing.T) {
	graph := context.NewResourceGraph()
	// node ← p1 ← svc1 ← ing1
	graph.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")
	graph.AddEdge(
		"ingress",
		"ns1",
		"ing1",
		"service",
		"ns1",
		"svc1",
		"routes_to",
	)

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource: "node",
			NodeName: "n1",
		},
	}
	ins := e.Analyze(inc)

	// Named, not counted: the reader needs to know *which* service and ingress.
	assert.Equal(
		t,
		"1 pods on this node, affecting service svc1 · ingress ing1",
		ins.Impact,
	)
}

func TestAnalyzeImpactConfigMapBlastRadius(t *testing.T) {
	graph := context.NewResourceGraph()
	// configmap ← p1 ← svc1, p2 ← svc1
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	graph.AddEdge("pod", "ns1", "p2", "configmap", "ns1", "cm1", "mounts")
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p2", "selects")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "configmap",
			Namespace: "ns1",
			Name:      "cm1",
		},
	}
	ins := e.Analyze(inc)

	// Both pods reference cm1 plus the service both are exposed through.
	assert.Equal(
		t,
		"2 pod(s) reference this configmap, affecting service svc1",
		ins.Impact,
	)
}

func TestAnalyzeImpactServiceAccountOnly(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "serviceaccount", "ns1", "sa1", "uses_sa")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "serviceaccount",
			Namespace: "ns1",
			Name:      "sa1",
		},
	}
	ins := e.Analyze(inc)

	assert.Equal(t, "1 pods", ins.Impact)
}

func TestAnalyzeRootCauseNodeViaPVChain(t *testing.T) {
	graph := context.NewResourceGraph()
	// pod attaches a PV which lives on a node — the failure originates at
	// node n1.
	graph.AddEdge("pod", "ns1", "p1", "persistentvolume", "", "pv-1", "binds")
	graph.AddEdge("persistentvolume", "", "pv-1", "node", "", "n1", "local_at")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
		},
	}
	ins := e.Analyze(inc)

	// "persistentvolume" is not a direct category match, so the transitive
	// walker finds the deepest suspect: the node.
	assert.Equal(t, "underlying node n1 may be unhealthy", ins.Cause)
	assert.Equal(t, "node_failure", ins.Pattern)
}

func TestAnalyzeRootCausePVCIncident(t *testing.T) {
	graph := context.NewResourceGraph()
	// pvc ← pv ← node; incident on the pvc itself.
	graph.AddEdge("pvc", "ns1", "pc1", "persistentvolume", "", "pv-9", "binds")
	graph.AddEdge("persistentvolume", "", "pv-9", "node", "", "n9", "local_to")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pvc",
			Namespace: "ns1",
			Name:      "pc1",
		},
	}
	ins := e.Analyze(inc)

	// pvc's direct deps: pv (not a category match) -> root walker blames node.
	assert.Equal(t, "underlying node n9 may be unhealthy", ins.Cause)
	assert.Equal(t, "node_failure", ins.Pattern)
}

func TestAnalyzeNoDeps(t *testing.T) {
	graph := context.NewResourceGraph()
	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "orphan",
		},
	}
	ins := e.Analyze(inc)

	assert.Empty(t, ins.Cause)
	assert.Empty(t, ins.Impact)
	assert.Empty(t, ins.Pattern)
}

func TestAnalyzePodIncidentKeyedByOwnerName(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "dep1-7d8d7", "node", "", "n1", "scheduled_on")
	graph.AddEdge(
		"pod",
		"ns1",
		"dep1-7d8d7",
		"deployment",
		"ns1",
		"dep1",
		"owned_by",
	)
	graph.AddEdge("pod", "ns1", "dep1-9c2a1", "node", "", "n1", "scheduled_on")
	graph.AddEdge(
		"pod",
		"ns1",
		"dep1-9c2a1",
		"deployment",
		"ns1",
		"dep1",
		"owned_by",
	)

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "dep1",
			OwnerKind: "Deployment",
			NodeName:  "n1",
		},
		Status: model.Status{
			Resources: map[string]bool{"dep1-7d8d7": true, "dep1-9c2a1": true},
		},
	}

	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "node n1")
	assert.Equal(t, "node_failure", ins.Pattern)
}

func TestAnalyzeWorkloadIncidentNamespaceName(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")
	graph.AddEdge("pod", "ns1", "p2", "deployment", "ns1", "dep1", "owned_by")
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "deployment",
			Namespace: "ns1",
			Name:      "ns1/dep1",
		},
	}
	ins := e.Analyze(inc)

	assert.Equal(
		t,
		"affects service svc1",
		ins.Impact,
	) // service reached transitively, and named
}
