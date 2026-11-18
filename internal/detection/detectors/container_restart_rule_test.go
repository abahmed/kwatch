package detectors

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ruleContainer is a container whose last exit had code after a run of
// run, in state (waiting in back-off, terminated or running again).
type ruleContainer struct {
	state    string
	code     float64
	reason   string
	restarts float64
	run      time.Duration
	byRule   bool
}

func (c ruleContainer) attrs(now time.Time) map[string]inventory.Value {
	finished := now.Add(-2 * time.Second)
	attrs := map[string]inventory.Value{
		kube.AttrState:        inventory.Text(c.state),
		kube.AttrRestarts:     inventory.Number(c.restarts),
		kube.AttrLastReason:   inventory.Text(c.reason),
		kube.AttrLastExitCode: inventory.Number(c.code),
		kube.AttrLastFinished: inventory.Time(finished),
		kube.AttrLastStarted:  inventory.Time(finished.Add(-c.run)),
	}
	switch c.state {
	case "waiting":
		attrs[kube.AttrStateReason] = inventory.Text(
			reasons.CrashLoopBackOff)
	case "terminated":
		attrs[kube.AttrStateReason] = inventory.Text(c.reason)
		attrs[kube.AttrExitCode] = inventory.Number(c.code)
		attrs[kube.AttrFinished] = inventory.Time(finished)
	case "running":
		attrs[kube.AttrStartedAt] = inventory.Time(now.Add(-time.Second))
	}
	if c.byRule {
		attrs[kube.AttrRestartRule] = inventory.Bool(true)
		attrs[kube.AttrRestartRuleRun] = inventory.Number(c.run.Seconds())
	}
	return attrs
}

func TestContainerRestartedByItsRuleIsNoCrash(t *testing.T) {
	long, short := 20*time.Minute, 3*time.Second
	tt := []struct {
		name string
		c    ruleContainer
		want string
	}{
		{"rule restart waiting in back-off", ruleContainer{"waiting", 42,
			"Error", 4, long, true}, ""},
		{"rule restart just exited", ruleContainer{"terminated", 42,
			"Error", 4, long, true}, ""},
		{"rule restart running again", ruleContainer{"running", 42,
			"Error", 6, long, true}, ""},
		{"first rule restarts on start", ruleContainer{"waiting", 42,
			"Error", 1, short, true}, ""},
		{"rule restarts on start again and again", ruleContainer{
			"waiting", 42, "Error", 4, short, true},
			reasons.CrashLoopBackOff},
		{"no rule matched", ruleContainer{"waiting", 1, "Error", 4, long,
			false}, reasons.CrashLoopBackOff},
		{"no rule matched just exited", ruleContainer{"terminated", 1,
			"Error", 4, long, false}, reasons.CrashLoopBackOff},
		{"memory kill is never excused", ruleContainer{"terminated", 137,
			reasons.OOMKilled, 1, long, true}, reasons.OOMKilled},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			now := t0
			e := buildContainer(inventory.EntityID{Kind: kube.KindPod,
				Namespace: "default", Name: "pod-1"}, "app", now,
				tc.c.attrs(now))
			found := Container{}.Detect(
				testDetectorContext(nil, now), e)
			got := ""
			if len(found) > 0 {
				got = found[0].Reason
			}
			if got != tc.want {
				t.Fatalf("reason %q, want %q: %+v", got, tc.want, found)
			}
		})
	}
}

// Containers that a RestartAllContainers rule stops while the pod
// restarts in place were stopped on purpose.
func TestContainerStoppedForAllContainersRestart(t *testing.T) {
	model := newTestModel()
	now := t0
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "pod-1"}
	setCondition(model, pod, allContainersRestarting, "True", "", "",
		now)
	id := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "default", Name: "pod-1/app"}
	_, err := model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: now, Entity: id,
		Attributes: ruleContainer{"terminated", 143, "Error", 1, time.Hour,
			false}.attrs(now)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := model.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: now, Entity: id,
		Relation: inventory.PartOf,
		Targets:  []inventory.EntityID{pod}}); err != nil {
		t.Fatal(err)
	}
	e, _ := model.Entity(id)
	ctx := detection.Context{Model: model, Now: now}
	if found := (Container{}).Detect(ctx, e); len(found) != 0 {
		t.Fatalf("stopped container reported: %+v", found)
	}
}
