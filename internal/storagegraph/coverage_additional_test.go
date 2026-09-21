package storagegraph

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func TestStorageGraphStatusAndSourceLifecycle(t *testing.T) {
	var nilMonitor *Monitor
	if got := nilMonitor.Status().State; got != "unavailable" {
		t.Fatalf("nil status = %q", got)
	}
	if nilMonitor.WaitForCacheSync(context.Background()) {
		t.Fatal("nil monitor reported cache sync")
	}

	monitor := &Monitor{}
	if got := monitor.Status().State; got != "unavailable" {
		t.Fatalf("initial status = %q", got)
	}
	if _, err := monitor.StatusJSON(); err != nil {
		t.Fatalf("status JSON: %v", err)
	}
	if err := monitor.ConfigureSources(Sources{
		Namespaces: []string{"apps"},
		WatchAll:   false,
	}); err != nil {
		t.Fatalf("configure sources: %v", err)
	}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second source configuration succeeded")
	}
	if got := monitor.watchNamespaces(true); len(got) != 1 || got[0] != "apps" {
		t.Fatalf("configured namespaces = %v", got)
	}
	if got := monitor.watchNamespaces(false); len(got) != 1 || got[0] != "" {
		t.Fatalf("cluster namespaces = %v", got)
	}
	if len(monitor.resourceSpecs()) != 5 {
		t.Fatalf("resource specs = %d", len(monitor.resourceSpecs()))
	}
	if err := monitor.Stop(context.Background()); err != nil {
		t.Fatalf("stop before start: %v", err)
	}
}

func TestStorageGraphProcessesHealthyAndErrorObjects(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}

	monitor.processCSIDriver(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "driver"},
		},
	})
	monitor.processCSIDriver(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "driver"},
			"status": map[string]interface{}{
				"error": map[string]interface{}{"message": "failed"},
			},
		},
	})
	monitor.processVolumeAttachment(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "healthy"},
		},
	})
	monitor.processVolumeAttachment(struct{}{})
	monitor.processVolumeSnapshot(struct{}{})
	monitor.processSnapshotContent(struct{}{})
	monitor.processSnapshotClass(struct{}{})

	content := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "content"},
		"spec": map[string]interface{}{
			"volumeSnapshotRef": map[string]interface{}{
				"name": "snapshot", "namespace": "apps",
			},
			"source": map[string]interface{}{"volumeHandle": "pv-1"},
		},
		"status": map[string]interface{}{
			"error": map[string]interface{}{"message": "failed"},
		},
	}}
	monitor.processSnapshotContent(content)
	class := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "class"},
		"driver":   "example.csi",
	}}
	monitor.processSnapshotClass(class)
	if len(graph.DependenciesOf(
		"volumesnapshotcontent", "", "content",
	)) == 0 {
		t.Fatal("snapshot content dependencies were not recorded")
	}
	if len(graph.DependenciesOf(
		"volumesnapshotclass", "", "class",
	)) != 1 {
		t.Fatal("snapshot class dependency was not recorded")
	}
}

func TestStorageGraphHelpersAndDeletionKinds(t *testing.T) {
	withMessage := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"attachError": map[string]interface{}{
				"reason": "Timeout", "message": "late",
			},
		},
	}}
	if !attachError(withMessage) || attachmentErrorHint(withMessage) !=
		"Timeout: late" {
		t.Fatal("attachment error was not decoded")
	}
	withoutMessage := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{
			"error": map[string]interface{}{"message": "snapshot failed"},
		},
	}}
	if !snapshotError(withoutMessage) ||
		snapshotErrorHint(withoutMessage) != "snapshot failed" {
		t.Fatal("snapshot error was not decoded")
	}
	if attachError(&unstructured.Unstructured{}) ||
		snapshotError(&unstructured.Unstructured{}) {
		t.Fatal("empty object reported an error")
	}

	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	for _, watched := range []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{volumeAttachmentGVR, "volumeattachment"},
		{volumeSnapshotGVR, "volumesnapshot"},
		{snapshotContentGVR, "volumesnapshotcontent"},
		{snapshotClassGVR, "volumesnapshotclass"},
		{csiDriverGVR, "csidriver"},
	} {
		graph.AddEdge(watched.kind, "", "object", "node", "", "n", "uses")
		monitor.removeNode(
			watched.gvr,
			&unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "object",
				},
			}},
		)
	}
	monitor.removeNode(volumeAttachmentGVR, cache.DeletedFinalStateUnknown{
		Key: "broken", Obj: struct{}{},
	})
}
