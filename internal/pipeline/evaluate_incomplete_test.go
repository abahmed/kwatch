package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// flakyDetector reports one failure, then panics on every later pass.
type flakyDetector struct{ calls int }

func (*flakyDetector) Name() string { return "flaky" }

func (*flakyDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}

func (d *flakyDetector) Detect(
	detection.Context, inventory.Entity,
) []detection.Finding {
	d.calls++
	if d.calls > 1 {
		panic("malformed object on a later pass")
	}
	return []detection.Finding{{
		Reason: "CrashLoopBackOff", Severity: detection.Critical}}
}

func TestEngineKeepsFindingsWhenADetectorPanics(t *testing.T) {
	flaky := &flakyDetector{}
	e := newTestEngine(t, &fakeClock{now: time.Now()}, func(
		context.Context, incident.Decision, notification.Message,
	) {
	}, func(deps *Dependencies) {
		deps.Detectors = detection.NewRegistry(nil, flaky)
	})
	id := inventory.CoreID(kube.KindPod, "shop", "web-0")
	now := time.Now()
	checks := newRechecks()
	dirty := e.apply([]inventory.Observation{seen(id, now, nil)})

	e.evaluate(context.Background(), now, dirty, checks)
	e.evaluate(context.Background(), now.Add(time.Second), dirty, checks)

	if got := e.tracker.Active(id); len(got) != 1 {
		t.Fatalf("active = %v: a panicking detector must not clear "+
			"the finding it reported before", got)
	}
	if checks.next().IsZero() {
		t.Fatal("an incomplete evaluation must ask for a recheck")
	}
}
