package detectors

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A pod whose init container is still running (a long migration) is
// doing its first job, not stuck Pending; once the init container has
// stopped, a pod that stays Pending is reported again.
func TestPodPendingWhileAnInitContainerRuns(t *testing.T) {
	for name, state := range map[string]string{
		"init running":    "running",
		"init terminated": "terminated",
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			pod := newID(kube.KindPod, "ns", "p")
			cond := kube.ConditionKey("PodScheduled")
			put(m, pod, t0, map[string]inventory.Value{
				kube.AttrPhase:                 inventory.Text("Pending"),
				kube.AttrCreated:               inventory.Time(t0),
				kube.AttrStartTime:             inventory.Time(t0),
				cond:                           inventory.Text("True"),
				cond + kube.AttrConditionSince: inventory.Time(t0),
			})
			c := newID(kube.KindContainer, "ns", "p/migrate")
			put(m, c, t0, map[string]inventory.Value{
				kube.AttrInit:      inventory.Bool(true),
				kube.AttrState:     inventory.Text(state),
				kube.AttrStartedAt: inventory.Time(t0),
			})
			relateEntity(m, c, inventory.PartOf, pod)
			got := evaluate(NewPod(PodThresholds{}), m,
				t0.Add(8*time.Minute), pod, nil)
			pending := false
			for _, f := range got.Findings {
				pending = pending || f.Reason == reasons.PodPending
			}
			if pending != (state != "running") {
				t.Fatalf("PodPending = %v with the init container %s",
					pending, state)
			}
		})
	}
}

// A sidecar that runs and has not passed its startup probe holds the
// pod Pending while it starts: that is the pod starting, judged by the
// probe's own budget, not a pod stuck Pending. A started sidecar, or
// one past its budget, does not hold the pod's wait.
func TestPodPendingWhileASidecarStarts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		starting bool
		budget   float64
		pending  bool
	}{
		{"sidecar within its startup budget", true, 900, false},
		{"sidecar past its startup budget", true, 300, true},
		{"sidecar started", false, 900, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel()
			pod := newID(kube.KindPod, "ns", "p")
			cond := kube.ConditionKey("PodScheduled")
			put(m, pod, t0, map[string]inventory.Value{
				kube.AttrPhase:                 inventory.Text("Pending"),
				kube.AttrCreated:               inventory.Time(t0),
				kube.AttrStartTime:             inventory.Time(t0),
				cond:                           inventory.Text("True"),
				cond + kube.AttrConditionSince: inventory.Time(t0),
			})
			c := newID(kube.KindContainer, "ns", "p/proxy")
			attrs := map[string]inventory.Value{
				kube.AttrSidecar:     inventory.Bool(true),
				kube.AttrState:       inventory.Text("running"),
				kube.AttrStartedAt:   inventory.Time(t0),
				kube.AttrProbeBudget: inventory.Number(tc.budget),
			}
			if tc.starting {
				attrs[kube.AttrSidecarStarting] = inventory.Bool(true)
			}
			put(m, c, t0, attrs)
			relateEntity(m, c, inventory.PartOf, pod)
			got := evaluate(NewPod(PodThresholds{}), m,
				t0.Add(8*time.Minute), pod, nil)
			pending := false
			for _, f := range got.Findings {
				pending = pending || f.Reason == reasons.PodPending
			}
			if pending != tc.pending {
				t.Fatalf("PodPending = %v, want %v", pending, tc.pending)
			}
		})
	}
}
