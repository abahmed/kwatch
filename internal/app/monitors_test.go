package app

import "testing"

func TestNewMonitorRegistryContainsRuntimeModules(t *testing.T) {
	registry, err := newMonitorRegistry()
	if err != nil {
		t.Fatalf("newMonitorRegistry() error = %v", err)
	}
	for _, name := range []string{
		"control-plane-monitor",
		"cluster-monitor",
		"network-monitor",
		"node-monitor",
		"pod-monitor",
		"probe-monitor",
		"security-monitor",
		"storage-monitor",
		"telemetry-monitor",
		"workload-monitor",
	} {
		descriptor, ok := registry.Lookup(name)
		if !ok {
			t.Fatalf("monitor %q is not registered", name)
		}
		if descriptor.Documentation == "" {
			t.Errorf("monitor %q has no documentation link", name)
		}
	}
}

func TestComposeIntegrationRuntimeDisablesControlPlane(t *testing.T) {
	integration, status := composeIntegrationRuntime(nil, nil, nil, nil)

	if integration.ControlPlane != nil {
		t.Fatal("disabled control-plane processor should remain nil")
	}
	if integration.ControlPlaneConfig != nil {
		t.Fatal("disabled control-plane configuration should remain nil")
	}
	if status != nil {
		t.Fatal("disabled control-plane status provider should remain nil")
	}
}
