//go:build e2e

package scenarios

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// resyncSeconds is a KwatchConfig field that deploy/crd.yaml defines, so the
// API server keeps it. The CRD already refuses a value kwatch would reject
// (0 or at least 30), so the invalid-overlay scenario below uses a spec the
// schema accepts but kwatch's semantic validation refuses.
const resyncSeconds = "resyncSeconds"

func TestScenarioConfigurationReload(t *testing.T) {
	onCluster(t, "lifecycle.configuration-reload", func(s *Scenario) {
		s.CreateKwatchConfig("kwatch-e2e-reload",
			map[string]any{resyncSeconds: int64(600)})
		s.ExpectKwatchHealthy()
	})
}

func TestScenarioInvalidLiveConfiguration(t *testing.T) {
	onCluster(t, "invalid-config.live-reload", func(s *Scenario) {
		s.CreateKwatchConfig("kwatch-e2e-invalid",
			map[string]any{
				// Allowed and forbidden namespaces cannot be mixed.
				"namespaces": []any{"default", "!kube-system"},
			})
		s.ExpectKwatchHealthy()
	})
}

func TestScenarioInvalidStartupConfiguration(t *testing.T) {
	onCluster(t, "invalid-config.startup", func(s *Scenario) {
		name := s.StartKwatchWithBrokenConfig()
		s.WaitForKwatchPod(name, corev1.PodFailed)
		s.ExpectNoPanicInLog(name)
	})
}
