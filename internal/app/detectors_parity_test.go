package app

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
)

// registryLayout lists, per kind, the types of the detectors a registry
// runs, in order. The registry keeps them in an unexported map; the test
// reads it with reflection so production code needs no accessor.
func registryLayout(r *detection.Registry) []string {
	byKind := reflect.ValueOf(r).Elem().FieldByName("byKind")
	var out []string
	for _, kind := range byKind.MapKeys() {
		list := byKind.MapIndex(kind)
		for i := 0; i < list.Len(); i++ {
			out = append(out, fmt.Sprintf("%v %d %s", kind, i,
				list.Index(i).Elem().Type()))
		}
	}
	sort.Strings(out)
	return out
}

// The scenario library, the pipeline and replay harnesses build their
// registries from detectors.Default. Production must too, or a detector
// added in one place would be scored but never run, or run unscored.
func TestProductionDetectorsAreDefault(t *testing.T) {
	got := registryLayout(newDetectorRegistry(nil))
	want := registryLayout(detection.NewRegistry(nil, detectors.Default()...))
	if len(want) == 0 {
		t.Fatal("the default registry holds no detectors")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("production detectors differ from detectors.Default:\n"+
			"got  %v\nwant %v", got, want)
	}
}
