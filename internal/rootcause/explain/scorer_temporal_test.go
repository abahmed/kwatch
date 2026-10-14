package explain

import (
	"testing"
	"time"
)

func TestScoreTemporalOrdersCauseBeforeEffect(t *testing.T) {
	for _, tc := range []struct {
		name        string
		nodeMinutes int
		want        float64
	}{
		{"cause first", 1, TemporalBefore},
		{"cause long after", 8, -TemporalAfter},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			node, pods := nodeWithPods(f, 2, "api")
			f.fail(node, "NotReady", failingH, tc.nodeMinutes, "")
			for _, pod := range pods {
				f.fail(pod, "NotReady", failingH, 3, "")
			}
			requireWeight(t, scoreOf(t, f, node, scoreTemporal), tc.want)
		})
	}
}

// After a restart, a detector finding is stamped with the time kwatch
// first saw it while pod findings keep their cluster times. That
// mixed clock must not read as "the cause began after the failures".
func TestScoreTemporalIgnoresObservationTimeAfterRestart(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 2, "api")
	f.fail(node, "NotReady", failingH, 30, "")
	for _, pod := range pods {
		f.fail(pod, "NotReady", failingH, 3, "")
	}
	s := f.snapshot()
	s.Started = t0.Add(30 * time.Minute)
	got := scoreSnapshot(t, s, node, scoreTemporal)
	requireWeight(t, got, 0)
}

// Both clocks agree before the restart floor: the verdict stands.
func TestScoreTemporalKeepsVerdictWhenBothPredateStart(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 2, "api")
	f.fail(node, "NotReady", failingH, 20, "")
	for _, pod := range pods {
		f.fail(pod, "NotReady", failingH, 3, "")
	}
	s := f.snapshot()
	s.Started = t0.Add(30 * time.Minute)
	got := scoreSnapshot(t, s, node, scoreTemporal)
	requireWeight(t, got, -TemporalAfter)
}

// Cause.Began never claims the restart as when the cause began.
func TestBeganOmitsObservationTimeFloor(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 2, "api")
	f.fail(node, "NotReady", failingH, 30, "")
	f.fail(pods[0], "NotReady", failingH, 3, "")
	s := f.snapshot()
	s.Started = t0.Add(30 * time.Minute)
	v := newView(s)
	if got := v.began(node); !got.IsZero() {
		t.Fatalf("began = %v, want unknown", got)
	}
	if got := v.began(pods[0]); got.IsZero() {
		t.Fatal("a cluster-time start must be kept")
	}
}

// A cause seen at start and failures seen live afterwards share one
// clock: the order is real and the verdict stands.
func TestScoreTemporalKeepsOrderOfFailuresSeenAfterStart(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 2, "api")
	f.fail(node, "NotReady", failingH, 31, "")
	for _, pod := range pods {
		f.fail(pod, "NotReady", failingH, 40, "")
	}
	s := f.snapshot()
	s.Started = t0.Add(30 * time.Minute)
	got := scoreSnapshot(t, s, node, scoreTemporal)
	requireWeight(t, got, TemporalBefore)
}

// A real cluster-time start a few minutes after kwatch began is not an
// observation stamp: a cause that began after the failures is still
// penalised.
func TestScoreTemporalPenalisesLateCauseInsideStartupMinutes(t *testing.T) {
	f := newFixture(t)
	node, pods := nodeWithPods(f, 2, "api")
	f.fail(node, "NotReady", failingH, 35, "")
	for _, pod := range pods {
		f.fail(pod, "NotReady", failingH, 20, "")
	}
	s := f.snapshot()
	s.Started = t0.Add(30 * time.Minute)
	got := scoreSnapshot(t, s, node, scoreTemporal)
	requireWeight(t, got, -TemporalAfter)
}
