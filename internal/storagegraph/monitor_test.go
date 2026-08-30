package storagegraph

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/context"
)

func TestProcessVolumeAttachmentBuildsFailureDependencies(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "va-1"},
		"spec": map[string]interface{}{
			"nodeName": "node-1",
			"attacher": "example.csi",
			"source":   map[string]interface{}{"persistentVolumeName": "pv-1"},
		},
		"status": map[string]interface{}{
			"attachError": map[string]interface{}{"message": "timed out"},
		},
	}}

	monitor.processVolumeAttachment(obj)

	assertContains(t, graph.DependenciesOf("volumeattachment", "", "va-1"), "persistentvolume//pv-1")
	assertContains(t, graph.DependenciesOf("volumeattachment", "", "va-1"), "node//node-1")
	assertContains(t, graph.DependenciesOf("volumeattachment", "", "va-1"), "csidriver//example.csi")
	assertContains(t, graph.DependenciesOf("volumeattachment", "", "va-1"), "volumeattachment_failure//va-1")
	assertContains(t, graph.DependenciesOf("persistentvolume", "", "pv-1"), "volumeattachment//va-1")
}

func TestRemoveNodeHandlesDeletionTombstone(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	graph.AddEdge("csidriver", "", "example.csi", "node", "", "node-1", "supports")
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "example.csi"},
	}}

	monitor.removeNode(csiDriverGVR, cache.DeletedFinalStateUnknown{Key: "example.csi", Obj: obj})

	if got := graph.DependenciesOf("csidriver", "", "example.csi"); len(got) != 0 {
		t.Fatalf("tombstone delete left graph dependencies: %v", got)
	}
}

func TestProcessVolumeSnapshotBuildsPVCAndFailureDependencies(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "snap-1", "namespace": "apps"},
		"spec": map[string]interface{}{
			"source": map[string]interface{}{"persistentVolumeClaimName": "data"},
		},
		"status": map[string]interface{}{
			"error": map[string]interface{}{"message": "backend rejected request"},
		},
	}}

	monitor.processVolumeSnapshot(obj)

	assertContains(t, graph.DependenciesOf("volumesnapshot", "apps", "snap-1"), "pvc/apps/data")
	assertContains(t, graph.DependenciesOf("volumesnapshot", "apps", "snap-1"), "volumesnapshot_failure/apps/snap-1")
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%q not found in %v", want, values)
}
