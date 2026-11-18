package detectors

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A kubelet with a short maximum back-off restarts a container almost
// at once, so a container that crashes fifty seconds after each start
// spends most of its loop running. The run between two crashes is held
// as part of the loop for twice the last run, not a fixed thirty
// seconds, and never longer than maxCrashLoopHold.
func TestCrashLoopHoldFollowsTheLastRun(t *testing.T) {
	for _, tc := range []struct {
		name    string
		lastRun time.Duration
		running time.Duration
		loop    bool
	}{
		{"crash on start, early in the run", 3 * time.Second,
			20 * time.Second, true},
		{"crash on start, run past the hold", 3 * time.Second,
			40 * time.Second, false},
		{"fifty-second runs, forty seconds in", 50 * time.Second,
			40 * time.Second, true},
		{"fifty-second runs, past twice that", 50 * time.Second,
			110 * time.Second, false},
		{"long runs are capped", 10 * time.Minute,
			6 * time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := t0
			started := now.Add(-tc.running)
			finished := started.Add(-10 * time.Second)
			e := buildContainer(inventory.EntityID{Kind: kube.KindPod,
				Namespace: "default", Name: "pod-1"}, "app", now,
				map[string]inventory.Value{
					kube.AttrState:        inventory.Text("running"),
					kube.AttrStartedAt:    inventory.Time(started),
					kube.AttrRestarts:     inventory.Number(6),
					kube.AttrLastReason:   inventory.Text("Error"),
					kube.AttrLastExitCode: inventory.Number(1),
					kube.AttrLastFinished: inventory.Time(finished),
					kube.AttrLastStarted: inventory.Time(
						finished.Add(-tc.lastRun)),
				})
			found := crashLoopRunning(testDetectorContext(nil, now), e)
			loop := len(found) == 1 &&
				found[0].Reason == reasons.CrashLoopBackOff
			if loop != tc.loop {
				t.Fatalf("crash loop = %v, want %v: %+v", loop, tc.loop,
					found)
			}
		})
	}
}
