package insight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	context "github.com/abahmed/kwatch/internal/graphcontext"
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
