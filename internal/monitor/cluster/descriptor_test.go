package cluster

import "testing"

func TestDescriptorDeclaresClusterMonitor(t *testing.T) {
	descriptor := Descriptor()
	if descriptor.Name != "cluster-monitor" {
		t.Fatalf("name = %q, want cluster-monitor", descriptor.Name)
	}
	if len(descriptor.Resources) == 0 {
		t.Fatal("descriptor has no monitored resources")
	}
}
