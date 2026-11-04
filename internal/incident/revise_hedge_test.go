package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// hedgeRig has one announced incident, api explained by cfg-a, that
// only a roll-up announced when listed.
func hedgeRig(t *testing.T, listed bool) (*rig, detection.Finding) {
	t.Helper()
	r := newRig(t, Config{})
	apiPod, _ := workload(r, "api")
	api := sig(apiPod, reasons.CrashLoopBackOff, detection.Warning)
	r.rule.causes[apiPod] = tieCause(cm("cfg-a"), 0.50, apiPod)
	r.raise(at(0), api)
	require.Len(t, r.tick(at(DefaultSettle)), 1, "api is announced")
	if listed {
		r.m.RecordRolledUp(r.idOf(cm("cfg-a")))
	}
	return r, api
}

// moveTo makes the solver explain api by to, weighing others, in the
// first minutes after the announcement, and returns what the ticks up
// to the end of the revise settle decided.
func moveTo(
	r *rig, api detection.Finding, to explain.Cause,
	others ...explain.Cause,
) []Decision {
	now := DefaultSettle + explain.TemporalExclusion/2
	r.rule.causes[api.Entity] = to
	r.rule.alternatives = map[inventory.EntityID][]explain.Cause{
		api.Entity: others}
	r.apply(at(now), detection.Changed, api)
	ds := r.tick(at(now))
	return append(ds, r.tick(at(now+DefaultReviseSettle+time.Second))...)
}

// A new cause that only ties with the cause a roll-up stated, the told
// root being its rival, moves the incident but sends no "cause changed":
// it would say "two possible causes: cfg-c or cfg-a" to people told
// cfg-a, and nothing observed contradicts them.
func TestManagerRevisionTiedWithListedCauseSaysNothing(t *testing.T) {
	r, api := hedgeRig(t, true)
	apiPod := api.Entity

	ds := moveTo(r, api, tieCause(cm("cfg-c"), 0.52, apiPod),
		tieCause(cm("cfg-a"), 0.50, apiPod))

	wantNone(t, ds)
	got := r.of(cm("cfg-c"))
	assert.Equal(t, 1, got.Revision, "no update was sent")
	require.NotNil(t, got.Cause)
	require.NotNil(t, got.Cause.Rival)
	assert.Equal(t, cm("cfg-a"), got.Cause.Rival.Root)
	wantNone(t, r.tick(at(DefaultSettle+time.Hour)))
}

// A new cause whose rival is something people were never told is a
// real revision.
func TestManagerRevisionTiedWithAnotherCauseIsSent(t *testing.T) {
	r, api := hedgeRig(t, true)
	apiPod := api.Entity

	ds := moveTo(r, api, tieCause(cm("cfg-c"), 0.52, apiPod),
		tieCause(cm("cfg-d"), 0.50, apiPod))

	wantAction(t, ds, Update, ReasonCauseRevised)
}

// A new cause that outscores the told one by more than the margin has
// no rival: the thread is told the cause changed.
func TestManagerRevisionPastTheToldCauseIsSent(t *testing.T) {
	r, api := hedgeRig(t, true)
	apiPod := api.Entity

	ds := moveTo(r, api, tieCause(cm("cfg-c"), 0.90, apiPod),
		tieCause(cm("cfg-a"), 0.50, apiPod))

	wantAction(t, ds, Update, ReasonCauseRevised)
}

// Two moves before the update goes out keep the root people heard: a
// tie with the unsent middle root is still news.
func TestManagerRevisionTiedWithUnsentRootIsSent(t *testing.T) {
	r, api := hedgeRig(t, true)
	apiPod := api.Entity
	now := DefaultSettle + explain.TemporalExclusion/2
	r.rule.causes[apiPod] = tieCause(cm("cfg-b"), 0.90, apiPod)
	r.apply(at(now), detection.Changed, api)
	wantNone(t, r.tick(at(now)))

	ds := moveTo(r, api, tieCause(cm("cfg-c"), 0.52, apiPod),
		tieCause(cm("cfg-b"), 0.50, apiPod))

	wantAction(t, ds, Update, ReasonCauseRevised)
}

// An incident with a thread of its own hears the same tie there: the
// new root is said in the conversation people already follow.
func TestManagerRevisionTiedWithOwnThreadCauseIsSent(t *testing.T) {
	r, api := hedgeRig(t, false)
	apiPod := api.Entity

	ds := moveTo(r, api, tieCause(cm("cfg-c"), 0.52, apiPod),
		tieCause(cm("cfg-a"), 0.50, apiPod))

	wantAction(t, ds, Update, ReasonCauseRevised)
}

// A skipped hedge keeps the tie in the fingerprint: when the told root
// drops out and the new cause stands alone, people told cfg-a hear of
// cfg-c once, and not again.
func TestManagerTieEndingAfterSkippedHedgeIsSent(t *testing.T) {
	r, api := hedgeRig(t, true)
	apiPod := api.Entity
	wantNone(t, moveTo(r, api, tieCause(cm("cfg-c"), 0.52, apiPod),
		tieCause(cm("cfg-a"), 0.50, apiPod)))

	now := DefaultSettle + explain.TemporalExclusion/2 +
		DefaultReviseSettle + time.Minute
	r.rule.causes[apiPod] = tieCause(cm("cfg-c"), 0.52, apiPod)
	r.rule.alternatives = nil
	r.apply(at(now), detection.Changed, api)
	var ds []Decision
	for i := range 5 {
		ds = append(ds, r.tick(at(now+time.Duration(i)*time.Minute))...)
	}

	require.Len(t, ds, 1, "one update tells the new cause")
	assert.Equal(t, Update, ds[0].Action)
	got := r.of(cm("cfg-c"))
	require.NotNil(t, got.Cause)
	assert.Nil(t, got.Cause.Rival)
	wantNone(t, r.tick(at(now+time.Hour)))
}
