package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestScheduleSuspended(t *testing.T) {
	tests := []struct {
		kind   knowledge.Kind
		reason string
	}{
		{kube.KindCronJob, constant.ReasonCronJobSuspended},
		{kube.KindJob, constant.ReasonJobSuspended},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			m := newTestModel()
			id := newID(tt.kind, "default", "j")
			put(m, id, t0, map[string]knowledge.Value{
				kube.AttrSuspended: knowledge.Bool(true),
			})
			got := evaluate(Schedule{}, m, t0, id, nil).Signals
			require.Len(t, got, 1)
			assert.Equal(t, tt.reason, got[0].Reason)
			assert.Equal(t, signal.Info, got[0].Severity)
			assert.NotEmpty(t, got[0].Summary)
		})
	}
}

func TestScheduleMissedRun(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindCronJob, "default", "nightly")
	next := t0.Add(time.Hour)
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrNextRun: knowledge.Time(next),
	})
	pending := evaluate(Schedule{}, m, next.Add(4*time.Minute), id, nil)
	assert.Empty(t, pending.Signals)
	assert.Equal(t, time.Minute, pending.RecheckAfter)

	due := evaluate(Schedule{}, m, next.Add(5*time.Minute), id, nil)
	require.Len(t, due.Signals, 1)
	assert.Equal(t, constant.ReasonCronJobNotScheduled,
		due.Signals[0].Reason)
	assert.Equal(t, signal.Warning, due.Signals[0].Severity)
	assert.True(t, next.Equal(due.Signals[0].Since))
	assert.NotEmpty(t, due.Signals[0].Summary)
}

func TestScheduleQuiet(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "default", "c")
	put(m, cron, t0, nil)
	assert.Empty(t, evaluate(Schedule{}, m, t0, cron, nil).Signals)

	job := newID(kube.KindJob, "default", "j")
	put(m, job, t0, map[string]knowledge.Value{
		kube.AttrNextRun: knowledge.Time(t0.Add(-time.Hour)),
	})
	assert.Empty(t, evaluate(Schedule{}, m, t0, job, nil).Signals,
		"only CronJobs have a next run")
	assert.Equal(t, "schedule", Schedule{}.Name())
	assert.Len(t, Schedule{}.Kinds(), 2)
}

func TestNamespaceTerminatingSustained(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNamespace, "", "gone")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrPhase: knowledge.Text("Terminating"),
	})
	early := evaluate(Namespace{}, m, t0.Add(9*time.Minute), id, nil)
	assert.Empty(t, early.Signals)
	assert.Equal(t, time.Minute, early.RecheckAfter)

	late := evaluate(Namespace{}, m, t0.Add(10*time.Minute), id, nil)
	require.Len(t, late.Signals, 1)
	assert.Equal(t, constant.ReasonNamespaceStuck, late.Signals[0].Reason)
	assert.Equal(t, signal.Warning, late.Signals[0].Severity)
	assert.NotEmpty(t, late.Signals[0].Summary)
}

func TestNamespacePodSecurityInvalid(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNamespace, "", "team")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrPhase:              knowledge.Text("Active"),
		kube.AttrPodSecurityInvalid: knowledge.Text("enforce=bogus"),
	})
	got := evaluate(Namespace{}, m, t0, id, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonPodSecurityPolicyInvalid, got[0].Reason)
	assert.Contains(t, got[0].Summary, "enforce=bogus")
}

func TestNamespaceQuiet(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNamespace, "", "ok")
	put(m, id, t0, map[string]knowledge.Value{
		kube.AttrPhase: knowledge.Text("Active"),
	})
	assert.Empty(t, evaluate(Namespace{}, m, t0, id, nil).Signals)
	assert.Equal(t, "namespace", Namespace{}.Name())
	assert.Len(t, Namespace{}.Kinds(), 2)
}

func TestNamespaceLimitRange(t *testing.T) {
	m := newTestModel()
	bad := newID(kube.KindLimitRange, "default", "bad")
	put(m, bad, t0, map[string]knowledge.Value{
		kube.AttrLimitsInvalid: knowledge.Text("min above max"),
	})
	got := evaluate(Namespace{}, m, t0, bad, nil).Signals
	require.Len(t, got, 1)
	assert.Equal(t, constant.ReasonLimitRangeInvalid, got[0].Reason)
	assert.Equal(t, signal.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "min above max")

	good := newID(kube.KindLimitRange, "default", "good")
	put(m, good, t0, nil)
	assert.Empty(t, evaluate(Namespace{}, m, t0, good, nil).Signals)
}
