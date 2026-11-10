package detectors

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// crashLoopRig is a container observed through the container detector
// and a tracker, as the pipeline sees it.
type crashLoopRig struct {
	t     *testing.T
	model *inventory.Model
	reg   *detection.Registry
	tr    *detection.Tracker
	id    inventory.EntityID
}

func newCrashLoopRig(t *testing.T) *crashLoopRig {
	return &crashLoopRig{t: t, model: newTestModel(),
		reg: detection.NewRegistry(nil, Container{}),
		tr:  detection.NewTracker(),
		id: inventory.EntityID{Kind: kube.KindContainer,
			Namespace: "default", Name: "pod-1/app"}}
}

// observe records the container at at: waiting with reason, or running
// since started, after restarts error exits.
func (r *crashLoopRig) observe(at time.Time, state, reason string,
	restarts float64, started time.Time,
) []detection.Transition {
	attrs := map[string]inventory.Value{
		kube.AttrState:        inventory.Text(state),
		kube.AttrRestarts:     inventory.Number(restarts),
		kube.AttrLastReason:   inventory.Text("Error"),
		kube.AttrLastExitCode: inventory.Number(1),
		kube.AttrLastFinished: inventory.Time(started.Add(-time.Second)),
	}
	if reason != "" {
		attrs[kube.AttrStateReason] = inventory.Text(reason)
	}
	if state == "running" {
		attrs[kube.AttrStartedAt] = inventory.Time(started)
	}
	_, err := r.model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: at, Entity: r.id, Attributes: attrs})
	if err != nil {
		r.t.Fatal(err)
	}
	return r.tr.Observe(r.id, r.reg.Evaluate(r.model, at, r.id).Findings)
}

func cleared(ts []detection.Transition) bool {
	for _, x := range ts {
		if x.Kind == detection.Cleared {
			return true
		}
	}
	return false
}

// A crash loop stays raised while the container runs for a moment
// between two back-offs, instead of clearing and coming back with each.
func TestCrashLoopKeptBetweenBackoffs(t *testing.T) {
	r := newCrashLoopRig(t)
	now := t0
	ts := r.observe(now, "waiting", reasons.CrashLoopBackOff, 3, now)
	if len(ts) != 1 || ts[0].Kind != detection.Raised {
		t.Fatalf("crash loop not raised: %+v", ts)
	}
	run := now.Add(40 * time.Second)
	if ts := r.observe(run, "running", "", 4, run); cleared(ts) {
		t.Fatalf("crash loop cleared while running between back-offs")
	}
	back := now.Add(50 * time.Second)
	for _, x := range r.observe(back, "waiting", reasons.CrashLoopBackOff,
		4, run) {
		if x.Kind == detection.Raised || x.Kind == detection.Cleared {
			t.Fatalf("crash loop raised again: %+v", x)
		}
	}
}

// A run that outlasts the longest back-off is a run, not a pause in the
// loop: with few restarts the crash loop then clears.
func TestCrashLoopClearsAfterALongRun(t *testing.T) {
	r := newCrashLoopRig(t)
	r.observe(t0, "waiting", reasons.CrashLoopBackOff, 3, t0)
	run := t0.Add(40 * time.Second)
	r.observe(run, "running", "", 4, run)
	later := run.Add(crashLoopHold + time.Second)
	if ts := r.observe(later, "running", "", 4, run); !cleared(ts) {
		t.Fatalf("crash loop kept after a long run: %+v", ts)
	}
}

// A loop reported early, at its first back-offs, is kept through the
// short runs before the restart count reaches crashLoopRestarts.
func TestCrashLoopKeptBeforeTheThirdRestart(t *testing.T) {
	r := newCrashLoopRig(t)
	r.observe(t0, "waiting", reasons.CrashLoopBackOff, 1, t0)
	run := t0.Add(20 * time.Second)
	if ts := r.observe(run, "running", "", 2, run); cleared(ts) {
		t.Fatalf("early crash loop cleared while running: %+v", ts)
	}
}

// One error exit and a run since is not a loop nobody reported.
func TestOneCrashThenARunIsNoCrashLoop(t *testing.T) {
	r := newCrashLoopRig(t)
	run := t0.Add(20 * time.Second)
	if ts := r.observe(run, "running", "", 1, run); len(ts) != 0 {
		t.Fatalf("a single crash raised a loop: %+v", ts)
	}
}
