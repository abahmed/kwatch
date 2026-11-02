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

// A CronJob watch that keeps failing holds the old next-run time: a run
// that looks overdue says nothing, so nothing is raised or cleared.
func TestScheduleIsUndecidedWhileTheCronJobWatchIsStale(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindCronJob, "default", "nightly")
	next := t0.Add(time.Hour)
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrNextRun: inventory.Time(next),
	})
	fresh := false
	r := detection.NewRegistry(nil, Schedule{})
	r.SetFresh(func(inventory.Kind) bool { return fresh })

	late := next.Add(10 * time.Minute)
	ev := r.Evaluate(m, late, id)
	assert.Empty(t, ev.Findings)
	assert.Equal(t, []string{"schedule"}, ev.UnsyncedBy)
	assert.Empty(t, ev.Unsynced)

	previous := []detection.Finding{{Entity: id,
		Reason: reasons.CronJobNotScheduled, Detector: "schedule"}}
	require.Len(t, ev.Keeping(previous), 1,
		"an earlier finding is kept, not cleared")

	fresh = true
	found := r.Evaluate(m, late, id).Findings
	require.Len(t, found, 1)
	assert.Equal(t, reasons.CronJobNotScheduled, found[0].Reason)
}
