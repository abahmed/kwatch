package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestScheduleSuspended(t *testing.T) {
	tests := []struct {
		kind   inventory.Kind
		reason string
	}{
		{kube.KindCronJob, reasons.CronJobSuspended},
		{kube.KindJob, reasons.JobSuspended},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			m := newTestModel()
			id := newID(tt.kind, "default", "j")
			put(m, id, t0, map[string]inventory.Value{
				kube.AttrSuspended: inventory.Bool(true),
			})
			got := evaluate(Schedule{}, m, t0, id, nil).Findings
			require.Len(t, got, 1)
			assert.Equal(t, tt.reason, got[0].Reason)
			assert.Equal(t, detection.Info, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestScheduleMissedRun(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindCronJob, "default", "nightly")
	next := t0.Add(time.Hour)
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrNextRun: inventory.Time(next),
	})
	pending := evaluate(Schedule{}, m, next.Add(4*time.Minute), id, nil)
	assert.Empty(t, pending.Findings)
	assert.Equal(t, time.Minute, pending.RecheckAfter)

	due := evaluate(Schedule{}, m, next.Add(5*time.Minute), id, nil)
	require.Len(t, due.Findings, 1)
	assert.Equal(t, reasons.CronJobNotScheduled,
		due.Findings[0].Reason)
	assert.Equal(t, detection.Warning, due.Findings[0].Severity)
	assert.True(t, next.Equal(due.Findings[0].Since))
	assert.NotEmpty(t, due.Findings[0].Summary)
}

func TestScheduleQuiet(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "default", "c")
	put(m, cron, t0, nil)
	assert.Empty(t, evaluate(Schedule{}, m, t0, cron, nil).Findings)

	job := newID(kube.KindJob, "default", "j")
	put(m, job, t0, map[string]inventory.Value{
		kube.AttrNextRun: inventory.Time(t0.Add(-time.Hour)),
	})
	assert.Empty(t, evaluate(Schedule{}, m, t0, job, nil).Findings,
		"only CronJobs have a next run")
	assert.Equal(t, "schedule", Schedule{}.Name())
	assert.Len(t, Schedule{}.Kinds(), 2)
}

func TestNamespaceTerminatingSustained(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNamespace, "", "gone")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Terminating"),
	})
	early := evaluate(Namespace{}, m, t0.Add(9*time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, time.Minute, early.RecheckAfter)

	late := evaluate(Namespace{}, m, t0.Add(10*time.Minute), id, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, reasons.NamespaceStuck, late.Findings[0].Reason)
	assert.Equal(t, detection.Warning, late.Findings[0].Severity)
	assert.NotEmpty(t, late.Findings[0].Summary)
}

func TestNamespacePodSecurityInvalid(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNamespace, "", "team")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrPhase:              inventory.Text("Active"),
		kube.AttrPodSecurityInvalid: inventory.Text("enforce=bogus"),
	})
	got := evaluate(Namespace{}, m, t0, id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.PodSecurityPolicyInvalid, got[0].Reason)
	assert.Contains(t, got[0].Summary, "enforce=bogus")
}

func TestNamespaceQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNamespace, "", "ok")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Active"),
	})
	assert.Empty(t, evaluate(Namespace{}, m, t0, id, nil).Findings)
	assert.Equal(t, "namespace", Namespace{}.Name())
	assert.Len(t, Namespace{}.Kinds(), 2)
}

func TestNamespaceLimitRange(t *testing.T) {
	m := newTestModel()
	bad := newID(kube.KindLimitRange, "default", "bad")
	put(m, bad, t0, map[string]inventory.Value{
		kube.AttrLimitsInvalid: inventory.Text("min above max"),
	})
	got := evaluate(Namespace{}, m, t0, bad, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.LimitRangeInvalid, got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "min above max")

	good := newID(kube.KindLimitRange, "default", "good")
	put(m, good, t0, nil)
	assert.Empty(t, evaluate(Namespace{}, m, t0, good, nil).Findings)
}

func TestScheduleInvalidCronJob(t *testing.T) {
	tests := []struct {
		name    string
		problem string
		want    int
	}{
		{"invalid_schedule", kube.ScheduleInvalid, 1},
		{"unknown_time_zone", kube.ScheduleBadZone, 1},
		{"no_problem", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			id := newID(kube.KindCronJob, "default", "j")
			attrs := map[string]inventory.Value{}
			if tt.problem != "" {
				attrs[kube.AttrScheduleProblem] = inventory.Text(tt.problem)
			}
			put(m, id, t0, attrs)
			got := evaluate(Schedule{}, m, t0, id, nil).Findings
			require.Len(t, got, tt.want)
			if tt.want == 1 {
				assert.Equal(t, reasons.CronJobInvalidSchedule,
					got[0].Reason)
			}
		})
	}
}
