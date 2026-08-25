package insight

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
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

	assert.Equal(t, "2 pods", ins.Impact)
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
		Resource: "pod", Namespace: "ns1", Name: "p1",
		NodeName: "n1",
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
	graph.AddEdge("ingress", "ns1", "ing1", "service", "ns1", "svc1", "routes_to")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "node", NodeName: "n1"}
	ins := e.Analyze(inc)

	assert.Equal(t, "1 pods on this node, affecting 1 services, 1 ingresses", ins.Impact)
}

func TestAnalyzeImpactConfigMapBlastRadius(t *testing.T) {
	graph := context.NewResourceGraph()
	// configmap ← p1 ← svc1, p2 ← svc1
	graph.AddEdge("pod", "ns1", "p1", "configmap", "ns1", "cm1", "mounts")
	graph.AddEdge("pod", "ns1", "p2", "configmap", "ns1", "cm1", "mounts")
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p1", "selects")
	graph.AddEdge("service", "ns1", "svc1", "pod", "ns1", "p2", "selects")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "configmap", Namespace: "ns1", Name: "cm1"}
	ins := e.Analyze(inc)

	// Both pods reference cm1 plus the service both are exposed through.
	assert.Equal(t, "2 pod(s) reference this configmap, affecting 1 services", ins.Impact)
}

func TestAnalyzeImpactServiceAccountOnly(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "serviceaccount", "ns1", "sa1", "uses_sa")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "serviceaccount", Namespace: "ns1", Name: "sa1"}
	ins := e.Analyze(inc)

	assert.Equal(t, "1 pods", ins.Impact)
}

func TestAnalyzeRootCauseNodeViaPVChain(t *testing.T) {
	graph := context.NewResourceGraph()
	// pod attaches a PV which lives on a node — the failure originates at node n1.
	graph.AddEdge("pod", "ns1", "p1", "persistentvolume", "", "pv-1", "binds")
	graph.AddEdge("persistentvolume", "", "pv-1", "node", "", "n1", "local_at")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{Resource: "pod", Namespace: "ns1", Name: "p1"}
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
	inc := &model.Incident{Resource: "pvc", Namespace: "ns1", Name: "pc1"}
	ins := e.Analyze(inc)

	// pvc's direct deps: pv (not a category match) -> root walker blames node.
	assert.Equal(t, "underlying node n9 may be unhealthy", ins.Cause)
	assert.Equal(t, "node_failure", ins.Pattern)
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

func TestAnalyzePodIncidentKeyedByOwnerName(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "dep1-7d8d7", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns1", "dep1-7d8d7", "deployment", "ns1", "dep1", "owned_by")
	graph.AddEdge("pod", "ns1", "dep1-9c2a1", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns1", "dep1-9c2a1", "deployment", "ns1", "dep1", "owned_by")

	e := NewEngine(graph, context.NewChangeTracker(10))
	inc := &model.Incident{
		Resource: "pod", Namespace: "ns1",
		Name: "dep1", OwnerKind: "Deployment", NodeName: "n1",
		Resources: map[string]bool{"dep1-7d8d7": true, "dep1-9c2a1": true},
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
	inc := &model.Incident{Resource: "deployment", Namespace: "ns1", Name: "ns1/dep1"}
	ins := e.Analyze(inc)

	assert.Equal(t, "affects 1 service(s)", ins.Impact) // service reached transitively
}
