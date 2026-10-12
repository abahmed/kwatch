//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestScenarioPodStartupFailureProfiles(t *testing.T) {
	inNamespace(t, "pod.startup-additional", func(s *Scenario) {
		s.CreatePod(podWithMissingImage("image-pull"))
		s.CreatePod(podWithFailingInit("init-container"))
		// PullNever makes the kubelet wait with ErrImageNeverPull, not
		// ImagePullBackOff.
		s.ExpectIncident("image-pull", "ErrImageNeverPull", 0)
		s.ExpectIncident("init-container", "CrashLoopBackOff", 0)
		s.ExpectKwatchHealthy()
	})
}

func TestScenarioPodReadinessFailure(t *testing.T) {
	inNamespace(t, "pod.not-ready", func(s *Scenario) {
		s.CreatePod(podNeverReady("not-ready"))
		// A fresh Kind cluster has nodes that joined together, which
		// kwatch reads as a wake-up: readiness failures of pods it
		// created wait until the wake-up is over (kube.WakeQuiet
		// after the last node joined).
		s.ExpectIncident("not-ready", "ReadinessProbeFailed",
			kube.WakeQuiet)
		s.ExpectKwatchHealthy()
	})
}
