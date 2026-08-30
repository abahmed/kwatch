package storagegraph

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%q not found in %v", want, values)
}
