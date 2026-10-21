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

// scriptedDetector reports the findings of its pass, and the last
// pass's again after the script ends.
type scriptedDetector struct {
	name   string
	passes [][]detection.Finding
	calls  int
}

func (d *scriptedDetector) Name() string { return d.name }

func (*scriptedDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}

func (d *scriptedDetector) Detect(
	detection.Context, inventory.Entity,
) []detection.Finding {
	pass := min(d.calls, len(d.passes)-1)
	d.calls++
	return append([]detection.Finding(nil), d.passes[pass]...)
}

func activeReasons(e *Engine, id inventory.EntityID) []string {
	var out []string
	for _, f := range e.tracker.Active(id) {
		out = append(out, f.Reason)
	}
	return out
}

// A detector that panics keeps its own earlier findings, but the other
// detectors of the entity are not frozen with it: what they found fresh
// is applied, including a finding that cleared and one that was raised.
func TestEngineAppliesOtherDetectorsWhenOneDetectorPanics(t *testing.T) {
	flaky := &flakyDetector{}
	steady := &scriptedDetector{name: "steady", passes: [][]detection.Finding{
		{{Reason: "NotReady", Severity: detection.Warning}},
		{{Reason: "ProbeFailed", Severity: detection.Warning}},
	}}
	e := newTestEngine(t, &fakeClock{now: time.Now()}, func(
		context.Context, incident.Decision, notification.Message,
	) {
	}, func(deps *Dependencies) {
		deps.Detectors = detection.NewRegistry(nil, flaky, steady)
	})
	id := inventory.CoreID(kube.KindPod, "shop", "web-0")
	now := time.Now()
	checks := newRechecks()
	dirty := e.apply([]inventory.Observation{seen(id, now, nil)})

	e.evaluate(context.Background(), now, dirty, checks)
	e.evaluate(context.Background(), now.Add(time.Second), dirty, checks)

	got := activeReasons(e, id)
	want := []string{"CrashLoopBackOff", "ProbeFailed"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("active = %v, want %v: the panicking detector keeps "+
			"its finding and the others' fresh results apply", got, want)
	}
}
