package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// introRig is an engine with two open incidents of one namespace, an
// investigator that answers at once, and a log of what reached the sink.
type introRig struct {
	e    *Engine
	sent []incident.Decision
	msgs []notification.Message
}

func newIntroRig(t *testing.T) *introRig {
	t.Helper()
	r := &introRig{}
	g := newGatedInvestigator()
	close(g.release)
	r.e = newTestEngine(t, &fakeClock{now: holdsNow},
		func(_ context.Context, d incident.Decision, m notification.Message) {
			r.sent = append(r.sent, d)
			r.msgs = append(r.msgs, m)
		}, func(d *Dependencies) { d.Investigator = g })
	startPool(t, r.e)
	mk := func(id, name string) incident.Record {
		return incident.Record{ID: id, Tier: incident.Notify,
			State: incident.Open, Opened: holdsNow.Add(-time.Minute),
			Root: inventory.EntityID{Kind: kube.KindDeployment,
				Namespace: "shop", Name: name}, PagedKnown: true}
	}
	r.e.deps.Incidents.Restore([]incident.Record{mk("a", "web"),
		mk("b", "api")}, time.Time{})
	return r
}

// rollUp announces both incidents in one tick: one roll-up names them.
func (r *introRig) rollUp(t *testing.T) {
	t.Helper()
	r.e.announcer.announce(context.Background(), holdsNow,
		[]incident.Decision{decisionOf(r.e, "a", incident.Announce),
			decisionOf(r.e, "b", incident.Announce)})
	require.NotEmpty(t, r.sent)
	require.Equal(t, incident.Reason("roll-up"), r.sent[0].Reason)
}

// investigate runs id's investigation to its end.
func (r *introRig) investigate(t *testing.T, id string) {
	t.Helper()
	for _, p := range r.e.deps.Incidents.Incidents() {
		if p.ID == id {
			require.True(t, r.e.announcer.investigate(p, holdsNow))
		}
	}
	r.e.announcer.attachOutput(context.Background(), receive(t, r.e))
}

func (r *introRig) introductions() []int {
	var out []int
	for i, d := range r.sent {
		if d.Reason == incident.ReasonIntroduced {
			out = append(out, i)
		}
	}
	return out
}

// A crash loop a roll-up only named says why once its investigation is
// in: one introduction with the evidence, chat only, never again.
func TestRolledUpIncidentIsIntroducedOnceWithItsEvidence(t *testing.T) {
	r := newIntroRig(t)
	r.rollUp(t)
	r.investigate(t, "a")

	r.e.announcer.announce(context.Background(), holdsNow.Add(time.Second), nil)
	r.e.announcer.announce(context.Background(), holdsNow.Add(time.Minute), nil)

	intros := r.introductions()
	require.Len(t, intros, 1, "one introduction, for a only")
	d, m := r.sent[intros[0]], r.msgs[intros[0]]
	assert.Equal(t, "a", d.Incident.ID)
	assert.Equal(t, incident.Update, d.Action)
	require.Len(t, d.Facts.Evidence, 1)
	assert.Equal(t, "error of a", d.Facts.Evidence[0].Text)
	assert.True(t, m.SkipPaging, "chat only")
}

// Without evidence in time there is nothing to introduce; an incident
// that already spoke on its own needs no introduction either.
func TestRolledUpIncidentWithoutEvidenceOrSpokeIsNotIntroduced(
	t *testing.T,
) {
	r := newIntroRig(t)
	r.rollUp(t)
	up := decisionOf(r.e, "a", incident.Update)
	up.Reason = incident.ReasonMaterialChange
	r.e.announcer.send(context.Background(), up, holdsNow)
	r.investigate(t, "a")

	r.e.announcer.announce(context.Background(),
		holdsNow.Add(outputWait+time.Second), nil)

	assert.Empty(t, r.introductions())
	assert.Empty(t, r.e.announcer.intros, "nothing left waiting")
}

// confirmingInvestigator confirms on a second look whatever it reads.
type confirmingInvestigator struct{ funcInvestigator }

func (confirmingInvestigator) Recorrelate(
	incident.Incident, []incident.Section,
) *incident.Correlation {
	return &incident.Correlation{Confirmed: "the last run ended OOMKilled"}
}

// A result stored before the incident had a cause is concluded again
// when the message is written; one that confirmed already is kept.
func TestMessageConcludesAgainOverAStoredReport(t *testing.T) {
	inv := confirmingInvestigator{}
	e := newTestEngine(t, &fakeClock{now: holdsNow},
		func(context.Context, incident.Decision, notification.Message) {},
		func(d *Dependencies) { d.Investigator = inv })
	a := e.announcer
	report := []incident.Section{{Check: incident.CheckPodState}}

	got := a.correlationNow(incident.Incident{ID: "a"},
		testResult{Report: report})
	require.NotNil(t, got)
	assert.Equal(t, "the last run ended OOMKilled", got.Confirmed)

	kept := &incident.Correlation{Confirmed: "read earlier"}
	got = a.correlationNow(incident.Incident{ID: "a"},
		testResult{Report: report, Correlation: kept})
	assert.Equal(t, "read earlier", got.Confirmed)
	assert.Nil(t, a.correlationNow(incident.Incident{ID: "a"}, testResult{}),
		"nothing read, nothing to conclude")
}
