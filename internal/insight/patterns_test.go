package insight

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
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
		graph.AddEdge("pod", "ns", fmt.Sprintf("p%d", i), "configmap", "ns", "cm1", "mounts")
	}

	th := dynamicThreshold("configmap/ns/cm1", graph)
	assert.Equal(t, 6, th) // 20 * 30% = 6
}

func TestDynamicThresholdPVC(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 10; i++ {
		graph.AddEdge("pod", "ns", fmt.Sprintf("p%d", i), "pvc", "ns", "pv1", "mounts")
	}

	th := dynamicThreshold("pvc/ns/pv1", graph)
	assert.Equal(t, 3, th) // 10 * 30% = 3
}

func TestDynamicThresholdSecret(t *testing.T) {
	graph := context.NewResourceGraph()
	for i := 0; i < 20; i++ {
		graph.AddEdge("pod", "ns", fmt.Sprintf("p%d", i), "secret", "ns", "s1", "mounts")
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
		graph.AddEdge("pod", "ns", fmt.Sprintf("p%d", i), "configmap", "ns", "cm1", "mounts")
	}

	incidents := make([]*model.Incident, 0)
	for i := 0; i < 3; i++ {
		incidents = append(incidents, &model.Incident{
			Resource:  "pod",
			Namespace: "ns",
			Name:      fmt.Sprintf("p%d", i),
			State:     model.StateActive,
		})
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
		{Resource: "pod", Namespace: "ns", Name: "p", State: model.StateActive},
	}

	mfs := ScanMassFailures(incidents, graph)
	// Only 1 incident, threshold for 5 dependents is 3 (5*30%=1, but floor is 3)
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
			Resource:  "pod",
			Namespace: "ns",
			Name:      "p",
			Reason:    "CrashLoopBackOff",
			State:     model.StateActive,
		})
	}

	mfs := ScanMassFailures(incidents, graph)

	if len(mfs) > 0 {
		assert.Equal(t, "node//n1", mfs[0].SharedDependency)
	}
}

func TestScanMassFailuresSkipsResolved(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns", "p1", "node", "", "n1", "scheduled_on")

	incidents := []*model.Incident{
		{Resource: "pod", Namespace: "ns", Name: "p1", State: model.StateResolved},
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
			Resource:  "pod",
			Namespace: "ns",
			Name:      "p",
			State:     model.StateActive,
		})
	}

	mfs := ScanMassFailures(incidents, graph)
	assert.GreaterOrEqual(t, len(mfs), 1) // at least one shared dep found
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
