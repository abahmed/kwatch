package insight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/constant"
	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

// Every pod references some ConfigMap; a reference alone must not be blamed.
func TestAnalyzeDoesNotBlameUnchangedConfigMap(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	e := NewEngine(graph, context.NewChangeTracker(10))

	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "pod", Namespace: "ns1", Name: "p1",
	}})

	assert.NotContains(t, ins.Cause, "ConfigMap")
	assert.NotContains(t, ins.Cause, "configmap")
	assert.NotEqual(t, "config_error", ins.Pattern)
}

// The same holds for the deepest-dependency fallback: a ConfigMap at the
// bottom of the chain is topology, not evidence, unless it changed.
func TestAnalyzeRootFallbackSkipsUnchangedSecret(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "serviceaccount", "ns1", "sa", "uses_sa")
	graph.AddEdge("serviceaccount", "ns1", "sa", "secret", "ns1", "tok", "x")
	e := NewEngine(graph, context.NewChangeTracker(10))

	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "pod", Namespace: "ns1", Name: "p1",
	}})

	assert.NotContains(t, ins.Cause, "secret tok")
}

func TestAnalyzeStaleConfigChangeIsNotBlamed(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := context.NewChangeTracker(10)
	tracker.Record(context.Change{
		Resource: "configmap", Namespace: "ns1", Name: "cm1",
		Type:      context.ChangeUpdate,
		Timestamp: now.Add(-dependencyChangeWindow - time.Minute),
	})
	e := NewEngine(graph, tracker)
	e.SetClock(func() time.Time { return now })

	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "pod", Namespace: "ns1", Name: "p1",
	}})

	assert.NotEqual(t, "config_error", ins.Pattern)
}

// A throttled container is a resource-limit problem whatever it mounts.
func TestAnalyzeReasonCauseOutranksGraph(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := context.NewChangeTracker(10)
	tracker.Record(context.Change{
		Resource: "configmap", Namespace: "ns1", Name: "cm1",
		Type: context.ChangeUpdate, Timestamp: now.Add(-time.Minute),
	})
	e := NewEngine(graph, tracker)
	e.SetClock(func() time.Time { return now })

	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "pod", Namespace: "ns1", Name: "p1",
		Reason: constant.ReasonContainerCPUThrottled,
	}})

	assert.Equal(t, "resource_limit", ins.Pattern)
	assert.Contains(t, ins.Cause, "CPU limit")
	assert.InDelta(t, 0.85, ins.Confidence, 0.16)
}

func TestAnalyzeHPAMetricsFailureIsNotDeploymentHealth(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge(
		"horizontalpodautoscaler", "ns1", "web",
		"deployment", "ns1", "web", "scales",
	)
	e := NewEngine(graph, context.NewChangeTracker(10))

	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "horizontalpodautoscaler", Namespace: "ns1",
		Name: "ns1/web", Reason: constant.ReasonFailedGetResourceMetric,
	}})

	assert.Equal(t, "metrics_unavailable", ins.Pattern)
	assert.NotContains(t, ins.Cause, "deployment")
}

// A node's heartbeat lease renews every ten seconds; it is never the cause.
func TestAnalyzeNodeIncidentNeverBlamesLease(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge(
		"node", "", "n1", "lease", "kube-node-lease", "n1", "heartbeat",
	)
	e := NewEngine(graph, context.NewChangeTracker(10))

	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "node", Name: "n1", NodeName: "n1",
		Reason: constant.ReasonNodePSIHigh,
	}})

	assert.NotContains(t, ins.Cause, "lease")
	assert.Equal(t, "node_pressure", ins.Pattern)
}

func TestAnalyzeLeaseIsNotARoot(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge(
		"node", "", "n1", "lease", "kube-node-lease", "n1", "heartbeat",
	)
	e := NewEngine(graph, context.NewChangeTracker(10))

	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "node", Name: "n1", NodeName: "n1",
		Reason: "NodeNotReady",
	}})

	assert.NotContains(t, ins.Cause, "lease")
}
