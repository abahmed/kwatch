package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// raiseStep sends a decision of incident "a" at tier and returns the
// message the sink got.
func raiseStep(
	t *testing.T, e *Engine, sent *[]notification.Message,
	action incident.Action, tier incident.Tier,
) notification.Message {
	t.Helper()
	d := incident.Decision{Action: action, Incident: incident.Incident{
		ID: "a", Tier: tier}}
	e.announcer.send(context.Background(), d, scopeNow)
	require.NotEmpty(t, *sent)
	return (*sent)[len(*sent)-1]
}

// flags is "raises keyed" of m.
func flags(m notification.Message) [2]bool {
	return [2]bool{m.RaisesPage, m.PageKeyed}
}

// An incident the pagers heard at notify that rises to Page raises the
// page once; every later message, the resolve too, goes to the page key.
// The resolve forgets the raise, so a reopen at notify starts fresh.
func TestNotifyThenPageRaisesThePageOnce(t *testing.T) {
	e, sent := scopeHarness(t)
	upd, page := incident.Update, incident.Page
	steps := []struct {
		action incident.Action
		tier   incident.Tier
		want   [2]bool
	}{
		{incident.Announce, incident.Notify, [2]bool{false, false}},
		{upd, page, [2]bool{true, true}},
		{upd, page, [2]bool{false, true}},
		{upd, incident.Notify, [2]bool{false, true}},
		{incident.Resolve, incident.Notify, [2]bool{false, true}},
		{incident.Announce, incident.Notify, [2]bool{false, false}},
		{incident.Resolve, incident.Notify, [2]bool{false, false}},
	}
	for i, s := range steps {
		m := raiseStep(t, e, sent, s.action, s.tier)
		assert.Equal(t, s.want, flags(m), "step %d", i)
	}
}

// An incident born at Page never gets the flags.
func TestBornAtPageNeverRaises(t *testing.T) {
	e, sent := scopeHarness(t)
	for _, a := range []incident.Action{
		incident.Announce, incident.Update, incident.Resolve,
	} {
		m := raiseStep(t, e, sent, a, incident.Page)
		assert.Equal(t, [2]bool{false, false}, flags(m), "%v", a)
	}
}

// A message the pagers never hear (chat only) neither records the tier
// below Page nor raises later.
func TestChatOnlyNotifyDoesNotArmTheRaise(t *testing.T) {
	e, sent := scopeHarness(t)
	d := incident.Decision{Action: incident.Announce,
		Reason:   incident.ReasonIntroduced,
		Incident: incident.Incident{ID: "a", Tier: incident.Notify}}
	e.announcer.send(context.Background(), d, scopeNow)
	m := raiseStep(t, e, sent, incident.Update, incident.Page)
	assert.Equal(t, [2]bool{false, false}, flags(m))
}
