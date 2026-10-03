//go:build e2e

package scenarios

import "testing"

func TestScenarioPodStartupFailureProfiles(t *testing.T) {
	inNamespace(t, "pod.startup-additional", func(s *Scenario) {
		s.Must(podsCreate(s.Ctx, s.Env, s.Namespace,
			podsMissingImage("image-pull"),
			podsFailingInit("init-container")))
		// PullNever makes the kubelet wait with ErrImageNeverPull, not
		// ImagePullBackOff.
		s.ExpectIncident("image-pull", "ErrImageNeverPull", 0)
		s.ExpectIncident("init-container", "CrashLoopBackOff", 0)
		s.ExpectKwatchHealthy()
	})
}

func TestScenarioPodReadinessFailure(t *testing.T) {
	inNamespace(t, "pod.not-ready", func(s *Scenario) {
		s.Must(podsCreate(s.Ctx, s.Env, s.Namespace,
			podsNeverReady("not-ready")))
		s.ExpectIncident("not-ready", "ReadinessProbeFailed", 0)
		s.ExpectKwatchHealthy()
	})
}
