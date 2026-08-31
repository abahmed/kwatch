package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTrimManagedFieldsPreservesDetectionMetadata(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "api",
			"namespace": "prod",
			"labels":    map[string]interface{}{"app": "api"},
			"annotations": map[string]interface{}{
				"kwatch.io/maintenance": "true",
			},
			"managedFields": []interface{}{map[string]interface{}{"manager": "kubectl"}},
		},
		"status": map[string]interface{}{"available": true},
	}}

	got, err := TrimManagedFields(obj)
	if err != nil {
		t.Fatalf("TrimManagedFields returned error: %v", err)
	}
	cleaned := got.(*unstructured.Unstructured)
	meta, err := meta.Accessor(cleaned)
	if err != nil {
		t.Fatalf("metadata accessor failed: %v", err)
	}
	if len(meta.GetManagedFields()) != 0 {
		t.Fatal("managedFields were not removed")
	}
	if meta.GetLabels()["app"] != "api" || meta.GetAnnotations()["kwatch.io/maintenance"] != "true" {
		t.Fatal("detection metadata was changed")
	}
	if cleaned.Object["status"].(map[string]interface{})["available"] != true {
		t.Fatal("status was changed")
	}
}

func BenchmarkTrimManagedFields(b *testing.B) {
	for i := 0; i < b.N; i++ {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":          "api",
				"managedFields": []interface{}{map[string]interface{}{"manager": "kubectl"}},
			},
		}}
		if _, err := TrimManagedFields(obj); err != nil {
			b.Fatal(err)
		}
	}
}
