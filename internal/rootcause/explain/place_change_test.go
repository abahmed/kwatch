package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A change of one setting names it; a change of several names none,
// unless it scaled the object to zero.
func TestPlaceOfChangeWithSeveralSettings(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	image := inventory.FieldChange{Path: "containers[app].image",
		Before: "api:1.0", After: "api:1.1"}
	env := inventory.FieldChange{Path: "containers[app].env.MODE",
		Before: "a", After: "b"}
	zero := inventory.FieldChange{Path: "spec.replicas", Before: "3",
		After: "0"}
	three := inventory.FieldChange{Path: "spec.replicas", Before: "1",
		After: "3"}
	change := func(fields ...inventory.FieldChange) inventory.Change {
		return inventory.Change{Entity: api, Fields: fields}
	}

	if p := placeOfChange(api, change(image)); p == nil ||
		p.Path != "image" {
		t.Fatalf("one setting: %+v", p)
	}
	if p := placeOfChange(api, change(image, env)); p != nil {
		t.Fatalf("image and env: %+v", p)
	}
	if p := placeOfChange(api, change(image, three)); p != nil {
		t.Fatalf("image and a scale up: %+v", p)
	}
	if p := placeOfChange(api, change(image, zero)); p == nil ||
		p.Path != "spec.replicas" || p.Value != "0" {
		t.Fatalf("scaled to zero: %+v", p)
	}
}
