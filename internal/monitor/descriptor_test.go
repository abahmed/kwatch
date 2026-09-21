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

func TestRegistryValidatesDescriptorsAndListsInOrder(t *testing.T) {
	base := Descriptor{
		Name:        "base",
		Description: "Base monitor",
		Feature:     feature.PodDetection,
	}
	for _, descriptor := range []Descriptor{
		{},
		{Name: "missing-description", Feature: feature.PodDetection},
		{Name: "missing-feature", Description: "missing"},
	} {
		if _, err := NewRegistry(descriptor); err == nil {
			t.Fatalf("descriptor %#v was accepted", descriptor)
		}
	}
	var nilRegistry *Registry
	if _, ok := nilRegistry.Lookup("missing"); ok || nilRegistry.List() != nil {
		t.Fatal("nil registry returned metadata")
	}
	r, err := NewRegistry(base, Descriptor{
		Name:        "alpha",
		Description: "Alpha monitor",
		Feature:     feature.NodeDetection,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := r.List()
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "base" {
		t.Fatalf("registry list = %#v", list)
	}
	list[0].Resources = append(list[0].Resources, "changed")
	if _, ok := r.Lookup("missing"); ok {
		t.Fatal("unknown descriptor was found")
	}
	var nilReceiver *Registry
	if err := nilReceiver.Register(base); err == nil {
		t.Fatal("nil registry accepted registration")
	}
}
