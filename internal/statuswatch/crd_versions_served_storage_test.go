package statuswatch

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

// TestWatchCRDOnlyTracksServedStorageVersion verifies that watchCRD tracks
// only versions that are both served and the storage version; a served but
// non-storage version must not add a second tracked version key.
func TestWatchCRDOnlyTracksServedStorageVersion(t *testing.T) {
	monitor := &Monitor{
		namespaceAllowed: func(namespace string) bool { return true },
		crdVersions:      make(map[string]map[string]struct{}),
		factories:        make(map[string]dynamicwatch.Factory),
		stops:            make(map[string]context.CancelFunc),
		versionDone:      make(map[string]chan struct{}),
	}
	monitor.watchAll = false
	monitor.namespaces = []string{"apps"}

	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "widgets.example.io"},
		"spec": map[string]interface{}{
			"group": "example.io", "scope": "Namespaced",
			"names": map[string]interface{}{"plural": "widgets"},
			"versions": []interface{}{
				map[string]interface{}{
					"name": "v1", "served": true, "storage": false,
					"subresources": map[string]interface{}{
						"status": map[string]interface{}{},
					},
				},
				map[string]interface{}{
					"name": "v2", "served": true, "storage": true,
					"subresources": map[string]interface{}{
						"status": map[string]interface{}{},
					},
				},
			},
		},
	}}

	monitor.watchCRD(crd)

	tracked := monitor.crdVersions["widgets.example.io"]
	if len(tracked) != 1 {
		t.Fatalf("tracked versions = %d, want 1", len(tracked))
	}
}
