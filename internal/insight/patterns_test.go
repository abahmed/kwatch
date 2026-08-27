package insight

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
)

func TestDynamicThresholdNilGraph(t *testing.T) {
	assert.Equal(t, minMassFailThreshold, dynamicThreshold("node//n1", nil))
}

func TestDynamicThresholdNode(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 10; i++ {
		graph.AddEdge("pod", "ns", "p", "node", "", "n1", "scheduled_on")
	}

	th := dynamicThreshold("node//n1", graph)
	assert.Equal(t, 3, th) // 10 * 30% = 3
}

func TestDynamicThresholdNodeSmall(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns", "p1", "node", "", "n1", "scheduled_on")

	th := dynamicThreshold("node//n1", graph)
	assert.Equal(t, minMassFailThreshold, th) // below minimum
}

func TestDynamicThresholdConfigMap(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 20; i++ {
		graph.AddEdge(
			"pod",
			"ns",
			fmt.Sprintf("p%d", i),
			"configmap",
			"ns",
			"cm1",
			"mounts",
		)
	}

	th := dynamicThreshold("configmap/ns/cm1", graph)
	assert.Equal(t, 6, th) // 20 * 30% = 6
}

func TestDynamicThresholdPVC(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 10; i++ {
		graph.AddEdge(
			"pod",
			"ns",
			fmt.Sprintf("p%d", i),
			"pvc",
			"ns",
			"pv1",
			"mounts",
		)
	}

	th := dynamicThreshold("pvc/ns/pv1", graph)
	assert.Equal(t, 3, th) // 10 * 30% = 3
}

func TestDynamicThresholdSecret(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 20; i++ {
		graph.AddEdge(
			"pod",
			"ns",
			fmt.Sprintf("p%d", i),
			"secret",
			"ns",
			"s1",
			"mounts",
		)
	}

	th := dynamicThreshold("secret/ns/s1", graph)
	assert.Equal(t, 6, th) // 20 * 30% = 6
}

func TestScanMassFailuresNilGraph(t *testing.T) {
	mfs := ScanMassFailures([]*model.Incident{{}}, nil)
	assert.Empty(t, mfs)
}

func TestScanMassFailuresBelowThreshold(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 14; i++ {
		graph.AddEdge(
			"pod",
			"ns",
			fmt.Sprintf("p%d", i),
			"configmap",
			"ns",
			"cm1",
			"mounts",
		)
	}

	incidents := make([]*model.Incident, 0)
	for i := 0; i < 3; i++ {
		incidents = append(incidents, &model.Incident{
			Subject: model.Subject{
				Resource:  "pod",
				Namespace: "ns",
				Name:      fmt.Sprintf("p%d", i),
			},
			Status: model.Status{
				State: model.StateActive,
			},
		},
		)
	}

	mfs := ScanMassFailures(incidents, graph)
	assert.Empty(t, mfs) // 3 incidents < threshold of 4 (14*30%=4)
}

func TestScanMassFailuresNoIncidents(t *testing.T) {
	graph := context.NewResourceGraph()
	mfs := ScanMassFailures(nil, graph)
	assert.Empty(t, mfs)

	mfs = ScanMassFailures([]*model.Incident{}, graph)
	assert.Empty(t, mfs)
}

func TestScanMassFailures(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 5; i++ {
		graph.AddEdge("pod", "ns", "p", "node", "", "n1", "scheduled_on")
	}

	incidents := []*model.Incident{
		{
			Subject: model.Subject{Resource: "pod", Namespace: "ns", Name: "p"},
			Status:  model.Status{State: model.StateActive},
		},
	}

	mfs := ScanMassFailures(incidents, graph)
	// Only 1 incident, threshold for 5 dependents is 3 (5*30%=1, but floor is
	// 3)
	assert.Empty(t, mfs)
}

func TestScanMassFailuresDetectsSharedDependency(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 5; i++ {
		podName := "p"
		graph.AddEdge("pod", "ns", podName, "node", "", "n1", "scheduled_on")
	}

	incidents := make([]*model.Incident, 0)
	for i := 0; i < 5; i++ {
		incidents = append(incidents, &model.Incident{
			Subject: model.Subject{
				Resource:  "pod",
				Namespace: "ns",
				Name:      "p",
				Reason:    "CrashLoopBackOff",
			},
			Status: model.Status{
				State: model.StateActive,
			},
		},
		)
	}

	mfs := ScanMassFailures(incidents, graph)

	// Five incidents on one node whose threshold is 3 (5 × 30% rounds below
	// the floor): exactly one mass failure, on that node.
	if assert.Len(t, mfs, 1) {
		assert.Equal(t, "node//n1", mfs[0].SharedDependency)
		assert.Equal(t, 5, mfs[0].AffectedCount)
		assert.Equal(t, 3, mfs[0].Threshold)
		assert.Equal(t, "CrashLoopBackOff", mfs[0].Reason)
	}
}

func TestScanMassFailuresSkipsResolved(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns", "p1", "node", "", "n1", "scheduled_on")

	incidents := []*model.Incident{
		{
			Subject: model.Subject{
				Resource:  "pod",
				Namespace: "ns",
				Name:      "p1",
			},
			Status: model.Status{State: model.StateResolved},
		},
	}

	mfs := ScanMassFailures(incidents, graph)
	assert.Empty(t, mfs)
}

func TestScanMassFailuresMultipleSharedDeps(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 5; i++ {
		podName := "p"
		graph.AddEdge("pod", "ns", podName, "node", "", "n1", "scheduled_on")
		graph.AddEdge("pod", "ns", podName, "configmap", "ns", "cm1", "mounts")
	}

	incidents := make([]*model.Incident, 0)
	for i := 0; i < 5; i++ {
		incidents = append(incidents, &model.Incident{
			Subject: model.Subject{
				Resource:  "pod",
				Namespace: "ns",
				Name:      "p",
			},
			Status: model.Status{
				State: model.StateActive,
			},
		},
		)
	}

	mfs := ScanMassFailures(incidents, graph)

	// Both shared dependencies cross their thresholds, so both are reported.
	// Results come from a map, so compare as a set.
	deps := make(map[string]int, len(mfs))
	for _, mf := range mfs {
		deps[mf.SharedDependency] = mf.AffectedCount
	}
	assert.Equal(t, map[string]int{"node//n1": 5, "configmap/ns/cm1": 5}, deps)
}

func TestDynamicThresholdNodeInnerIf(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns", "p1", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns", "p2", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns", "p3", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns", "p4", "node", "", "n1", "scheduled_on")

	th := dynamicThreshold("node//n1", graph)
	assert.Equal(t, 3, th) // 4*30% = 1, bumped to min 3
}

func TestDynamicThresholdConfigMapInnerIf(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns", "p1", "configmap", "ns", "cm1", "mounts")
	graph.AddEdge("pod", "ns", "p2", "configmap", "ns", "cm1", "mounts")
	graph.AddEdge("pod", "ns", "p3", "configmap", "ns", "cm1", "mounts")
	graph.AddEdge("pod", "ns", "p4", "configmap", "ns", "cm1", "mounts")

	th := dynamicThreshold("configmap/ns/cm1", graph)
	assert.Equal(t, 3, th) // 4*30% = 1, bumped to min 3
}

func TestDynamicThresholdUnknownPrefix(t *testing.T) {
	graph := context.NewResourceGraph()
	th := dynamicThreshold("foo//bar", graph)
	assert.Equal(t, minMassFailThreshold, th)
}

func TestDynamicThresholdConfigMapBelowMin(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns", "p1", "configmap", "ns", "cm1", "mounts")

	th := dynamicThreshold("configmap/ns/cm1", graph)
	assert.Equal(t, minMassFailThreshold, th)
}

func TestScanMassFailuresPodIncidentUsesResources(t *testing.T) {
	graph := context.NewResourceGraph()
	// 4 pods on the same node, all owned by dep1.
	for i := 0; i < 4; i++ {
		podName := fmt.Sprintf("dep1-7d8d7-%d", i)
		graph.AddEdge("pod", "ns", podName, "node", "", "n1", "scheduled_on")
		graph.AddEdge(
			"pod",
			"ns",
			podName,
			"deployment",
			"ns",
			"dep1",
			"owned_by",
		)
	}

	incidents := make([]*model.Incident, 0)
	for i := 0; i < 4; i++ {
		incidents = append(incidents, &model.Incident{
			Subject: model.Subject{
				Resource:  "pod",
				Namespace: "ns",
				Name:      "dep1",
				Reason:    "CrashLoopBackOff",
			},
			Status: model.Status{
				State: model.StateActive,
				Resources: map[string]bool{
					fmt.Sprintf("dep1-7d8d7-%d", i): true,
				},
			},
		},
		)
	}

	mfs := ScanMassFailures(incidents, graph)
	if assert.Len(t, mfs, 2) {
		deps := map[string]int{}
		for _, mf := range mfs {
			deps[mf.SharedDependency] = mf.AffectedCount
		}
		assert.Equal(t, 4, deps["node//n1"])
		assert.Equal(t, 4, deps["deployment/ns/dep1"])
	}
}

func TestScanMassFailuresWorkloadIncidentNamespaceName(t *testing.T) {
	graph := context.NewResourceGraph()
	// 5 deployment incidents, each tied to the same service via its pods.
	for i := 0; i < 5; i++ {
		depName := fmt.Sprintf("dep%d", i)
		podName := fmt.Sprintf("dep%d-pod", i)
		graph.AddEdge(
			"pod",
			"ns",
			podName,
			"deployment",
			"ns",
			depName,
			"owned_by",
		)
		graph.AddEdge("service", "ns", "svc1", "pod", "ns", podName, "selects")
	}

	incidents := make([]*model.Incident, 0)
	for i := 0; i < 5; i++ {
		depName := fmt.Sprintf("dep%d", i)
		incidents = append(incidents, &model.Incident{
			Subject: model.Subject{
				Resource:  "deployment",
				Namespace: "ns",
				Name:      "ns/" + depName,
			},
			Status: model.Status{
				State: model.StateActive,
			},
		},
		)
	}

	mfs := ScanMassFailures(incidents, graph)
	// Without the fix this produced deployment/ns/ns/depN keys and found
	// nothing.
	// Deployments have no shared *dependency*: the service selects pods, not
	// deployments.
	assert.Empty(t, mfs)
}

func TestMassFailureDescribe(t *testing.T) {
	mf := MassFailure{
		SharedDependency: "node//n1",
		AffectedCount:    5,
		Threshold:        3,
		Reason:           "CrashLoopBackOff",
		Namespace:        "ns",
		ResourceKind:     "pod",
	}
	desc := mf.Describe()
	assert.Contains(t, desc, "5")
	assert.Contains(t, desc, "pod")
	assert.Contains(t, desc, "/n1")
}

func TestMassFailureDescribeEnriched(t *testing.T) {
	mf := MassFailure{
		SharedDependency: "configmap/ns/cm1",
		AffectedCount:    4,
		Threshold:        3,
		Reason:           "CrashLoopBackOff",
		Namespace:        "ns",
		ResourceKind:     "pod",
		RootCause: "underlying node n1 may be unhealthy (pattern: " +
			"node_failure)",
		RecentChanges: []context.Change{
			{
				Resource:  "configmap",
				Namespace: "ns",
				Name:      "cm1",
				Type:      context.ChangeUpdate,
				Timestamp: time.Now().Add(-2 * time.Minute),
			},
		},
	}
	desc := mf.Describe()
	assert.Contains(t, desc, "root cause: underlying node n1")
	assert.Contains(t, desc, "recent changes")
	assert.Contains(t, desc, "cm1")
}

func TestMassFailureDescribeClampsFutureTimestamps(t *testing.T) {
	mf := MassFailure{
		SharedDependency: "configmap/ns/cm1",
		AffectedCount:    4,
		Threshold:        3,
		Reason:           "CrashLoopBackOff",
		Namespace:        "ns",
		ResourceKind:     "pod",
		RecentChanges: []context.Change{
			// Future timestamp (clock skew): must render as 0s, never "-Xs".
			{
				Resource:  "configmap",
				Namespace: "ns",
				Name:      "cm1",
				Type:      context.ChangeUpdate,
				Timestamp: time.Now().Add(5 * time.Minute),
			},
		},
	}
	desc := mf.Describe()
	assert.Contains(t, desc, "recent changes: ns/cm1 updated 0s ago")
	assert.NotContains(t, desc, "-")
}

func TestEnrichMassFailureRootCause(t *testing.T) {
	graph := context.NewResourceGraph()
	// configmap/ns/cm1 <- pod p1. cm1 is a leaf, so walking up from cm1 lands
	// on cm1 itself as the root.
	graph.AddEdge("pod", "ns", "p1", "configmap", "ns", "cm1", "mounts")

	tracker := context.NewChangeTracker(0)
	tracker.Record(context.Change{
		Resource:  "configmap",
		Namespace: "ns",
		Name:      "cm1",
		Type:      context.ChangeUpdate,
		Timestamp: time.Now().Add(-30 * time.Second),
	})

	e := NewEngine(graph, tracker)
	mf := e.EnrichMassFailure(MassFailure{
		SharedDependency: "configmap/ns/cm1",
		AffectedCount:    4,
		Threshold:        3,
	})
	assert.Contains(
		t,
		mf.RootCause,
		"underlying configmap cm1 may be changed or misconfigured",
	)
	assert.Contains(t, mf.RootCause, "config_error")
	assert.Len(t, mf.RecentChanges, 1)
	assert.Equal(t, "cm1", mf.RecentChanges[0].Name)
}

func TestEnrichMassFailureDeepRoot(t *testing.T) {
	graph := context.NewResourceGraph()
	// pv-1 <- node n1 and pv-1 <- pod p1; the shared dep is pv-1, whose
	// upstream
	// is the node — the walker should surface n1 as the root cause.
	graph.AddEdge("persistentvolume", "", "pv-1", "node", "", "n1", "local_at")
	graph.AddEdge("pod", "ns", "p1", "persistentvolume", "", "pv-1", "binds")

	e := NewEngine(graph, context.NewChangeTracker(0))
	mf := e.EnrichMassFailure(MassFailure{
		SharedDependency: "persistentvolume//pv-1",
		AffectedCount:    4,
		Threshold:        3,
	})
	assert.Contains(t, mf.RootCause, "underlying node n1 may be unhealthy")
	assert.Contains(t, mf.RootCause, "node_failure")
}
