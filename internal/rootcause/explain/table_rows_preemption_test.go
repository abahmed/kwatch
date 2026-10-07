package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// preemptedWorkers builds a healthy importer pod and a worker Deployment
// whose first n pods the scheduler preempted for it. It returns the
// victims.
func preemptedWorkers(f *fixture, n int) []inventory.EntityID {
	f.workload("batch", "importer", 1)
	f.workload("shop", "worker", 3)
	var victims []inventory.EntityID
	for i := 0; i < n; i++ {
		pod := inventory.CoreID(kube.KindPod, "shop",
			"worker-1-"+string(rune('0'+i)))
		f.findings[pod] = append(f.findings[pod], detection.Finding{
			Entity: pod, Reason: "PodPreempted",
			Mode: detection.ModePreempted, Health: degradedH,
			Since: t0.Add(2 * time.Minute), Summary: "preempted",
			Evidence: []detection.Evidence{{
				Label: detection.EvidencePreemptor,
				Value: "batch/importer-1-0"}},
		})
		victims = append(victims, pod)
	}
	return victims
}

var preemptionRowCases = []rowCase{
	{row: "preemptor", want: "deployment/batch/importer",
		build: func(f *fixture) inventory.EntityID {
			return preemptedWorkers(f, 1)[0]
		}},
}

// Many victims of one preemptor are one cause, rooted at the preemptor's
// workload.
func TestPreemptionOfManyPodsIsOneCause(t *testing.T) {
	f := newFixture(t)
	victims := preemptedWorkers(f, 3)

	e := f.explain()

	for _, victim := range victims {
		requireCause(t, e, victim, "deployment/batch/importer")
	}
	if got := causes(e); len(got) != 1 {
		t.Fatalf("causes = %v, want only the preemptor", got)
	}
}

// A preemptor that names no pod the model holds explains nothing.
func TestPreemptionWithoutAKnownPreemptorHasNoCause(t *testing.T) {
	f := newFixture(t)
	victims := preemptedWorkers(f, 1)
	f.findings[victims[0]][0].Evidence[0].Value = "batch/gone"

	if c, ok := f.explain().CauseOf(victims[0]); ok &&
		c.Row == "preemptor" {
		t.Fatalf("unknown preemptor must not be blamed: %+v", c)
	}
}
