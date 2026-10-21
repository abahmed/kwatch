package explain

import (
	"testing"
	"time"
)

// A finalizer is not an object kwatch watches, so it has no start of its
// own: it began when the first leftover it holds did. Without a start the
// incident layer cannot tell a late takeover from the true cause.
func TestPseudoCauseBeganWithTheFirstFailureItExplains(t *testing.T) {
	f := newFixture(t)
	stuck := f.stuck("ops", 3, widgetFinalizer)
	f.findings[stuck[2]][0].Since = t0.Add(-40 * 24 * time.Hour)

	c := requireCause(t, f.explain(), stuck[0],
		"finalizer/ops/"+widgetFinalizer)

	want := t0.Add(-40 * 24 * time.Hour)
	if !c.Began.Equal(want) {
		t.Fatalf("Began = %v, want %v", c.Began, want)
	}
}

// A pseudo cause seen only at kwatch's start is dated by the cluster, or
// not at all: the observation time says when kwatch noticed.
func TestPseudoCauseDoesNotBeginAtKwatchStart(t *testing.T) {
	f := newFixture(t)
	f.stuck("ops", 2, widgetFinalizer)
	s := f.snapshot()
	s.Started = t0.Add(-10 * 24 * time.Hour)

	e := Explain(s)

	for _, area := range e.Areas {
		for _, c := range area.Causes {
			if !c.Began.IsZero() {
				t.Fatalf("Began = %v, want unknown", c.Began)
			}
		}
	}
}
