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

// cronRun is jobRun's failed run with its Job owned by a CronJob that
// last succeeded at success (zero: never).
func cronRun(success time.Time) (
	m *inventory.Model, job, pod, container inventory.Entity,
) {
	model, p, c := jobRun("Failed")
	jobID := newID(kube.KindJob, "ns", "j")
	cron := newID(kube.KindCronJob, "ns", "nightly")
	attrs := map[string]inventory.Value{}
	if !success.IsZero() {
		attrs[kube.AttrLastSuccess] = inventory.Time(success)
	}
	put(model, cron, t0, attrs)
	link(model, jobID, inventory.OwnedBy, cron)
	j, _ := model.Entity(jobID)
	return model, j, p, c
}

// The failed Job of a CronJob stays in the API until the next failure
// replaces it. Once a later run succeeded it is history, with its pods
// and containers; before that, and for a Job no CronJob owns, it is
// reported.
func TestFailedRunIsHistoryAfterLaterSuccess(t *testing.T) {
	failedAt := t0
	cases := map[string]struct {
		success time.Time
		want    []string
	}{
		"a later run succeeded": {failedAt.Add(5 * time.Minute),
			[]string{}},
		"the last success is older than the failure": {
			failedAt.Add(-time.Hour), []string{reasons.JobFailed,
				reasons.PodFailed, reasons.Error}},
		"it never succeeded": {time.Time{}, []string{reasons.JobFailed,
			reasons.PodFailed, reasons.Error}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model, job, pod, container := cronRun(tc.success)
			ctx := testDetectorContext(model, failedAt.Add(10*time.Minute))
			var got []detection.Finding
			got = append(got, Job{}.Detect(ctx, job)...)
			got = append(got, NewPod(PodThresholds{}).Detect(ctx, pod)...)
			got = append(got, Container{}.Detect(ctx, container)...)
			assert.Equal(t, tc.want, findingReasons(got))
		})
	}
}

// A failure without a time cannot be placed before a success.
func TestFailedRunWithoutTimeIsNotSuperseded(t *testing.T) {
	model, _, _, _ := cronRun(t0.Add(time.Hour))
	id := newID(kube.KindJob, "ns", "j")
	put(model, id, t0, map[string]inventory.Value{
		kube.ConditionKey("Failed"): inventory.Text("True"),
	})
	job, _ := model.Entity(id)
	job.Attributes = withoutKey(job.Attributes,
		kube.ConditionKey("Failed")+kube.AttrConditionSince)
	assert.False(t, runSuperseded(
		testDetectorContext(model, t0.Add(2*time.Hour)), job))
}

func withoutKey(
	attrs map[string]inventory.Attribute, key string,
) map[string]inventory.Attribute {
	out := map[string]inventory.Attribute{}
	for k, v := range attrs {
		if k != key {
			out[k] = v
		}
	}
	return out
}
