package insight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
)

func TestAnalyzeNoGraph(t *testing.T) {
	e := NewEngine(nil, nil)
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
		},
	}
	ins := e.Analyze(inc)
	assert.Empty(t, ins.Cause)
	assert.Empty(t, ins.Impact)
}

func TestAnalyzeNodeFailure(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
			NodeName:  "n1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "node n1")
	assert.Equal(t, "node_failure", ins.Pattern)
}

func TestAnalyzeRootCausePrefersRecentDependencyChange(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "service", "ns1", "api", "routes_to")
	graph.AddEdge("service", "ns1", "api", "configmap", "ns1", "old", "backed_by")
	graph.AddEdge("service", "ns1", "api", "secret", "ns1", "new", "backed_by")
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	tracker := context.NewChangeTracker(10)
	tracker.Record(context.Change{Resource: "configmap", Namespace: "ns1", Name: "old", Type: context.ChangeUpdate, Timestamp: now.Add(-9 * time.Minute)})
	tracker.Record(context.Change{Resource: "secret", Namespace: "ns1", Name: "new", Type: context.ChangeUpdate, Timestamp: now.Add(-30 * time.Second)})

	e := NewEngine(graph, tracker)
	e.now = func() time.Time { return now }
	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "pod", Namespace: "ns1", Name: "p1",
	}})

	assert.Contains(t, ins.Cause, "secret new")
}

func TestAnalyzeRolloutFailure(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
			OwnerKind: "Deployment",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "Deployment")
	assert.Equal(t, "rollout_failure", ins.Pattern)
}

func TestAnalyzeConfigError(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "ConfigMap")
	assert.Equal(t, "config_error", ins.Pattern)
}

func TestAnalyzeSecretError(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "secret", "ns1", "s1", "env_from")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "Secret")
	assert.Equal(t, "config_error", ins.Pattern)
}

func TestAnalyzeImpactNode(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns1", "p2", "node", "", "n1", "scheduled_on")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource: "node",
			NodeName: "n1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "2 pods")
}

func TestAnalyzeImpactWorkload(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")
	graph.AddEdge("pod", "ns1", "p2", "deployment", "ns1", "dep1", "owned_by")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "deployment",
			Namespace: "ns1",
			Name:      "dep1",
		},
	}
	ins := e.Analyze(inc)

	assert.Equal(t, "2 pods", ins.Impact)
}

// A pod's own create/update is the incident, not what caused it. Reporting
// "pod p1 created 3m ago" under "what changed" for an alert about p1 says
// nothing, so it is ignored — the owner's rollout is what matters.
func TestAnalyzeRecentChangesIgnoresThePodItself(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	tracker.Record(context.Change{
		Resource: "pod", Namespace: "ns1", Name: "p1",
		Type: context.ChangeCreate, Timestamp: time.Now(),
	})

	e := NewEngine(nil, tracker)
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
		},
	}
	ins := e.Analyze(inc)

	assert.Empty(
		t,
		ins.RecentChanges,
		"the pod's own churn is the symptom, not the cause",
	)
}

// The owning Deployment being updated just before its pods fail is the single
// most useful thing an alert can say: it is a rollout.
func TestAnalyzeRecentChangesBlamesTheRollout(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	tracker.Record(context.Change{
		Resource: "deployment", Namespace: "ns1", Name: "api",
		Type: context.ChangeUpdate, Timestamp: time.Now().Add(-2 * time.Minute),
	})
	tracker.Record(context.Change{ // unrelated churn in the same namespace
		Resource: "pod", Namespace: "ns1", Name: "other-xyz",
		Type: context.ChangeCreate, Timestamp: time.Now(),
	})

	e := NewEngine(nil, tracker)
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "api",
			OwnerKind: "Deployment",
		},
		Status: model.Status{
			Resources: map[string]bool{"api-abc": true},
		},
	}

	ins := e.Analyze(inc)

	assert.Len(t, ins.RecentChanges, 1)
	assert.Equal(t, "api", ins.RecentChanges[0].Name)
	assert.Equal(t, "rollout", ins.Pattern)
	assert.Contains(t, ins.Cause, "Deployment ns1/api was updated")
	assert.Contains(t, ins.Cause, "rollout")
}

// A non-pod resource's own change is still relevant: a ConfigMap alert
// about a ConfigMap that was just edited.
func TestAnalyzeRecentChangesDirectForNonPod(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	tracker.Record(context.Change{
		Resource: "configmap", Namespace: "ns1", Name: "cfg",
		Type: context.ChangeUpdate, Timestamp: time.Now(),
	})
	e := NewEngine(nil, tracker)
	ins := e.Analyze(&model.Incident{
		Subject: model.Subject{
			Resource:  "configmap",
			Namespace: "ns1",
			Name:      "cfg",
		},
	})
	assert.Len(t, ins.RecentChanges, 1)
}

func TestAnalyzePVCError(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "pvc", "ns1", "pvc1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "PVC")
	assert.Equal(t, "config_error", ins.Pattern)
}

func TestAnalyzeImpactPodService(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
			NodeName:  "n1",
		},
	}
	ins := e.Analyze(inc)

	// Name the service — a reader acts on "svc1", not on "1 service".
	assert.Equal(t, "affects service svc1", ins.Impact)
}

func TestAnalyzeImpactConfigMap(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	graph.AddEdge("pod", "ns1", "p2", "configmap", "ns1", "cm1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "configmap",
			Namespace: "ns1",
			Name:      "cm1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "2 pod")
}

func TestAnalyzeImpactSecret(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "secret", "ns1", "s1", "env_from")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "secret",
			Namespace: "ns1",
			Name:      "s1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "1 pod")
}

func TestAnalyzeImpactPVC(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "pvc", "ns1", "pv1", "mounts")
	graph.AddEdge("pod", "ns1", "p2", "pvc", "ns1", "pv1", "mounts")
	graph.AddEdge("pod", "ns1", "p3", "pvc", "ns1", "pv1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pvc",
			Namespace: "ns1",
			Name:      "pv1",
		},
	}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "3 pod")
}

// Unrelated churn in the same namespace is not "what changed". It used to be
// reported as a fallback, which made every alert in a busy namespace list
// some other pod's deletion as if it mattered.
func TestAnalyzeRecentChangesIgnoresUnrelatedNamespaceChurn(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	tracker.Record(context.Change{
		Resource: "pod", Namespace: "ns1", Name: "p2",
		Type: context.ChangeDelete, Timestamp: time.Now(),
	})

	e := NewEngine(nil, tracker)
	inc := &model.Incident{
		Subject: model.Subject{
			Resource:  "pod",
			Namespace: "ns1",
			Name:      "p1",
		},
	}
	ins := e.Analyze(inc)

	assert.Empty(
		t,
		ins.RecentChanges,
		"another pod's deletion says nothing about this one",
	)
}

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
			Timestamp: now.Add(time.Duration(i) * time.Second),
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
