package monitor

import (
	"testing"

	"github.com/abahmed/kwatch/internal/feature"
)

func TestRegistryRejectsDuplicateMonitorNames(t *testing.T) {
	first := Descriptor{
		Name:        "pod-failures",
		Description: "Detect pod failures",
		Feature:     feature.PodDetection,
	}
	if _, err := NewRegistry(first, first); err == nil {
		t.Fatal("expected duplicate monitor names to be rejected")
	}
}

func TestRegistryRejectsUnknownFeature(t *testing.T) {
	_, err := NewRegistry(Descriptor{
		Name:        "unknown",
		Description: "Unknown capability",
		Feature:     feature.ID("missing.feature"),
	})
	if err == nil {
		t.Fatal("expected unknown feature to be rejected")
	}
}

func TestRegistryReturnsIndependentResourceSlices(t *testing.T) {
	r, err := NewRegistry(Descriptor{
		Name:        "pod-failures",
		Description: "Detect pod failures",
		Feature:     feature.PodDetection,
		Resources:   []string{"pod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, ok := r.Lookup("pod-failures")
	if !ok {
		t.Fatal("registered monitor was not found")
	}
	first.Resources[0] = "changed"
	second, ok := r.Lookup("pod-failures")
	if !ok || second.Resources[0] != "pod" {
		t.Fatalf("registry returned mutable internal data: %#v", second)
	}
}
