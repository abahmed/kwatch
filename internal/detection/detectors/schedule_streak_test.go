package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The failure began with the first failed run of the streak, not with
// the latest one.
func TestScheduleRepeatedFailureSinceTheStreakBegan(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "etl")
	put(m, cron, t0, nil)
	finishedRun(m, cron, "etl-1", t0, false)
	for i, name := range []string{"etl-2", "etl-3", "etl-4"} {
		finishedRun(m, cron, name, t0.Add(time.Duration(i+1)*time.Hour),
			true)
	}

	eval := evaluate(Schedule{}, m, t0.Add(5*time.Hour), cron, nil)

	require.Len(t, eval.Findings, 1)
	assert.True(t, t0.Add(time.Hour).Equal(eval.Findings[0].Since),
		"since %v", eval.Findings[0].Since)
}

// The Jobs of the first failures are gone: the streak began after the
// last success.
func TestScheduleRepeatedFailureSinceTheLastSuccess(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "backup")
	success := t0.Add(-30 * 24 * time.Hour)
	put(m, cron, t0, map[string]inventory.Value{
		kube.AttrRunsSinceSuccess: inventory.Number(29),
		kube.AttrLastSuccess:      inventory.Time(success),
	})
	finishedRun(m, cron, "backup-29", t0, true)

	eval := evaluate(Schedule{}, m, t0.Add(time.Hour), cron, nil)

	require.Len(t, eval.Findings, 1)
	assert.True(t, success.Equal(eval.Findings[0].Since),
		"since %v", eval.Findings[0].Since)
}
