package statuswatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

func TestCustomResourceKindLowercasesKind(t *testing.T) {
	u := &unstructured.Unstructured{}
	u.SetKind("Certificate")

	if got := customResourceKind(u); got != "certificate" {
		t.Fatalf("expected %q, got %q", "certificate", got)
	}
}

func TestCustomResourceKindDefaultsWhenMissing(t *testing.T) {
	if got := customResourceKind(nil); got != "customresource" {
		t.Fatalf("expected %q for nil object, got %q",
			"customresource", got)
	}

	empty := &unstructured.Unstructured{}
	if got := customResourceKind(empty); got != "customresource" {
		t.Fatalf("expected %q for empty kind, got %q",
			"customresource", got)
	}
}

func TestDeletedObjectUnwrapsTombstone(t *testing.T) {
	u := &unstructured.Unstructured{}
	u.SetKind("Certificate")
	u.SetName("example")

	tombstone := cache.DeletedFinalStateUnknown{Obj: u}

	got := deletedObject(tombstone)
	if got != u {
		t.Fatalf("expected unwrapped object %+v, got %+v", u, got)
	}
}

func TestDeletedObjectPassesThroughPlainObject(t *testing.T) {
	u := &unstructured.Unstructured{}
	u.SetKind("Issuer")

	if got := deletedObject(u); got != u {
		t.Fatalf("expected same object returned, got %+v", got)
	}
}
