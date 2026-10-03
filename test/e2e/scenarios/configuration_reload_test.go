//go:build e2e

package scenarios

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// resyncSeconds is a KwatchConfig field that deploy/crd.yaml defines, so the
// API server keeps it. Kwatch accepts 0 or more and rejects a negative value
// (internal/config/validate_semantic.go).
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
			map[string]any{resyncSeconds: int64(-1)})
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
