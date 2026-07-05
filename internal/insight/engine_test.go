package insight

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestAnalyzeNoGraph(t *testing.T) {
	e := NewEngine(nil, nil)
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1"}
	ins := e.Analyze(inc)
	assert.Empty(t, ins.Cause)
	assert.Empty(t, ins.Impact)
}

func TestAnalyzeNodeFailure(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1", NodeName: "n1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "node n1")
	assert.Equal(t, "node_failure", ins.Pattern)
}

func TestAnalyzeRolloutFailure(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1", OwnerKind: "Deployment"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "Deployment")
	assert.Equal(t, "rollout_failure", ins.Pattern)
}

func TestAnalyzeConfigError(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "ConfigMap")
	assert.Equal(t, "config_error", ins.Pattern)
}

func TestAnalyzeSecretError(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "secret", "ns1", "s1", "env_from")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "Secret")
	assert.Equal(t, "config_error", ins.Pattern)
}

func TestAnalyzeImpactNode(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns1", "p2", "node", "", "n1", "scheduled_on")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "node", NodeName: "n1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "2 pods")
}

func TestAnalyzeImpactWorkload(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "dep1", "owned_by")
	graph.AddEdge("pod", "ns1", "p2", "deployment", "ns1", "dep1", "owned_by")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "deployment", Namespace: "ns1", Name: "dep1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "2 dependent")
}

func TestAnalyzeRecentChanges(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	tracker.Record(context.Change{
		Resource: "pod", Namespace: "ns1", Name: "p1",
		Type: context.ChangeCreate, Timestamp: time.Now(),
	})

	e := NewEngine(nil, tracker)
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1"}
	ins := e.Analyze(inc)

	assert.Len(t, ins.RecentChanges, 1)
	assert.Equal(t, "p1", ins.RecentChanges[0].Name)
}

func TestAnalyzePVCError(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "pvc", "ns1", "pvc1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Cause, "PVC")
	assert.Equal(t, "config_error", ins.Pattern)
}

func TestAnalyzeImpactPodService(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1", NodeName: "n1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "1 service")
}

func TestAnalyzeImpactConfigMap(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	graph.AddEdge("pod", "ns1", "p2", "configmap", "ns1", "cm1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "configmap", Namespace: "ns1", Name: "cm1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "2 pod")
}

func TestAnalyzeImpactSecret(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "secret", "ns1", "s1", "env_from")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "secret", Namespace: "ns1", Name: "s1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "1 pod")
}

func TestAnalyzeImpactPVC(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "pvc", "ns1", "pv1", "mounts")
	graph.AddEdge("pod", "ns1", "p2", "pvc", "ns1", "pv1", "mounts")
	graph.AddEdge("pod", "ns1", "p3", "pvc", "ns1", "pv1", "mounts")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pvc", Namespace: "ns1", Name: "pv1"}
	ins := e.Analyze(inc)

	assert.Contains(t, ins.Impact, "3 pod")
}

func TestAnalyzeRecentChangesNamespaceFallback(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	tracker.Record(context.Change{
		Resource: "pod", Namespace: "ns1", Name: "p2",
		Type: context.ChangeDelete, Timestamp: time.Now(),
	})

	e := NewEngine(nil, tracker)
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1"}
	ins := e.Analyze(inc)

	assert.Len(t, ins.RecentChanges, 1)
	assert.Equal(t, "p2", ins.RecentChanges[0].Name)
}

func TestAnalyzeRecentChangesCap(t *testing.T) {
	tracker := context.NewChangeTracker(100)
	now := time.Now()
	for i := 0; i < 5; i++ {
		tracker.Record(context.Change{
			Resource: "pod", Namespace: "ns1", Name: fmt.Sprintf("p%d", i),
			Type: context.ChangeCreate, Timestamp: now,
		})
	}

	e := NewEngine(nil, tracker)
	inc := &model.Incident{Resource: "deploy", Namespace: "ns1", Name: "dep1"}
	ins := e.Analyze(inc)

	assert.Len(t, ins.RecentChanges, 3)
}

func TestAnalyzeNoDeps(t *testing.T) {
	graph := context.NewResourceGraph()
	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "orphan"}
	ins := e.Analyze(inc)

	assert.Empty(t, ins.Cause)
	assert.Empty(t, ins.Impact)
	assert.Empty(t, ins.Pattern)
}
