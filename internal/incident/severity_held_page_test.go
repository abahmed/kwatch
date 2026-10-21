package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// A severity override makes a crash loop critical, so it pages. When it
// resolves and fails again inside RepageWindow the repeat is held at
// notify like any repeated page: the override raises the severity, it
// does not undo the hold.
func TestSeverityOverrideDoesNotDefeatAHeldRepeatPage(t *testing.T) {
	r := newRig(t, Config{SeverityByReason: map[string]string{
		reasons.CrashLoopBackOff: "critical"}})
	pod, _ := workload(r, "web")
	web := sig(pod, reasons.CrashLoopBackOff, detection.Warning)
	r.raise(at(0), web)
	first := r.tick(at(DefaultSettle))
	wantAction(t, first, Announce, ReasonSettled)
	require.Equal(t, Page, first[0].Incident.Tier, "setup: override pages")
	now := at(DefaultSettle + 10*time.Second)
	r.clear(now, web)
	r.tick(now)
	now = now.Add(DefaultHold + time.Second)
	require.Equal(t, Resolve, r.tick(now)[0].Action)

	now = now.Add(time.Minute)
	r.raise(now, web)
	var told []Decision
	for end := now.Add(2 * time.Minute); now.Before(end); now =
		now.Add(10 * time.Second) {
		told = append(told, r.tick(now)...)
	}

	require.Len(t, told, 1, "one failing again")
	assert.Equal(t, ReasonFailingAgain, told[0].Reason)
	assert.Equal(t, Notify, told[0].Incident.Tier)
	assert.True(t, told[0].Incident.Delivery.PageHeld())
}
