//go:build e2e

package scenarios

import "testing"

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
		s.ExpectIncident("not-ready", "ReadinessProbeFailed", 0)
		s.ExpectKwatchHealthy()
	})
}
