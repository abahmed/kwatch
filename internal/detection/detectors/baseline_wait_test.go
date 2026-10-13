package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A pod that has not started is quoted beside how long the workload's
// pods usually take, and the finding is left as it was.
func TestPendingPodIsQuotedBesideTheUsualWait(t *testing.T) {
	r := newBaselineRig(nil)
	b := r.m.Baselines()
	start := t0.Truncate(time.Hour).Add(-30 * time.Hour)
	b.Touch(r.dep, start)
	for h := 1; h <= 20; h++ {
		b.Add(r.dep, inventory.MetricPendingSeconds, 8,
			start.Add(time.Duration(h)*time.Hour))
	}
	pod := entityOf(r.m, r.pod)

	got := withPendingBaseline(testDetectorContext(r.m, t0), pod,
		[]detection.Finding{{Reason: "PodPending"}})

	assert.Equal(t, detection.NormalUnjudged, got[0].Normal)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: detection.EvidenceBaseline,
		Value: "pending for 1h vs a usual 8s"})
}

func TestPendingBaselineSkipsAPodThatStarted(t *testing.T) {
	r := newBaselineRig(nil)
	put(r.m, r.pod, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrCreated:   inventory.Time(t0.Add(-time.Hour)),
		kube.AttrStartTime: inventory.Time(t0.Add(-59 * time.Minute)),
	})

	got := withPendingBaseline(testDetectorContext(r.m, t0),
		entityOf(r.m, r.pod), []detection.Finding{{Reason: "PodPending"}})

	assert.Empty(t, got[0].Evidence)
}

func TestRunningJobIsQuotedBesideItsUsualRun(t *testing.T) {
	r := newBaselineRig(nil)
	start := t0.Truncate(time.Hour).Add(-30 * time.Hour)
	r.m.Baselines().Touch(r.dep, start)
	for h := 1; h <= 20; h++ {
		r.m.Baselines().Add(r.dep, inventory.MetricJobSeconds, 480,
			start.Add(time.Duration(h)*time.Hour))
	}
	job := entityOf(r.m, r.dep)

	j := jobJudgement(testDetectorContext(r.m, t0), job,
		t0.Add(-2*time.Hour))

	assert.Equal(t, "running 2h vs a usual 8m", j.phrase)
}
