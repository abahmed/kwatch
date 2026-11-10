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
