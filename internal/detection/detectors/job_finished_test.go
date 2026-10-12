package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// jobRun builds a Failed pod with one terminated container. When
// jobState is not empty the pod is owned by a Job in that state
// ("Complete", "Failed" or "running"); otherwise it is standalone.
func jobRun(jobState string) (
	*inventory.Model, inventory.Entity, inventory.Entity,
) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model := newTestModel()
	pod := newID(kube.KindPod, "ns", "p")
	container := newID(kube.KindContainer, "ns", "p/c")
	put(model, pod, now, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Failed"),
	})
	put(model, container, now, map[string]inventory.Value{
		kube.AttrState:    inventory.Text("terminated"),
		kube.AttrExitCode: inventory.Number(1),
	})
	link(model, container, inventory.PartOf, pod)
	if jobState != "" {
		job := newID(kube.KindJob, "ns", "j")
		put(model, job, now, nil)
		if jobState != "running" {
			setCondition(model, job, jobState, "True", "", "", now)
		}
		link(model, pod, inventory.OwnedBy, job)
	}
	p, _ := model.Entity(pod)
	c, _ := model.Entity(container)
	return model, p, c
}

func TestJobRunFindings(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)
	cases := map[string]struct {
		job  string
		want bool
	}{
		"complete job hides the failed run": {"Complete", false},
		"running job keeps the failed run":  {"running", true},
		"failed job keeps the failed run":   {"Failed", true},
		"standalone failed pod stays":       {"", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model, pod, container := jobRun(tc.job)
			ctx := testDetectorContext(model, now)
			var got []detection.Finding
			got = append(got, NewPod(PodThresholds{}).Detect(ctx, pod)...)
			got = append(got, Container{}.Detect(ctx, container)...)
			if !tc.want {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, []string{reasons.PodFailed, reasons.Error},
				findingReasons(got))
		})
	}
}
