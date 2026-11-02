package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

var formerAt = time.Date(2026, 9, 29, 10, 6, 0, 0, time.UTC)

// formerIncident is a restored incident rooted at deployment cart whose
// cause moved away from node n1 (its former root), with events as its
// timeline.
func formerIncident(
	t *testing.T, former inventory.EntityID, events ...incident.Event,
) incident.Incident {
	t.Helper()
	m := incident.NewManager(incident.Config{}, nil)
	m.Restore([]incident.Record{{ID: "inc-1", Root: movedCart,
		Tier: incident.Page, State: incident.Open, Opened: formerAt,
		Announced: formerAt, Revision: 2, FormerRoot: former,
		RerootedAt: formerAt, Timeline: events}}, time.Time{})
	all := m.Incidents()
	require.Len(t, all, 1)
	return all[0]
}

func formerNodeRecovered() incident.Event {
	id := movedNode
	return incident.Event{At: formerAt, Entity: &id,
		Text: incident.RecoveredPrefix + "Node is NotReady (node n1)"}
}

func cartFailing() []detection.Finding {
	return []detection.Finding{{Entity: movedCart,
		Reason: "CrashLooping", Severity: detection.Critical,
		Since: formerAt, Summary: "Deployment is crash looping"}}
}

func TestFormerRootSentencesSayTheFormerRootRecovered(t *testing.T) {
	p := formerIncident(t, movedNode, formerNodeRecovered())
	p.Pending.MarkReplaced()

	got := formerRootSentences(caseFacts{p: p, members: cartFailing(),
		now: formerAt})

	require.Len(t, got, 1)
	assert.Equal(t, partLead, got[0].part)
	assert.Contains(t, got[0].text, "recovered")
	assert.Contains(t, got[0].text, "n1")
}

func TestFormerRootSentencesNeedAReplacedCause(t *testing.T) {
	p := formerIncident(t, movedNode, formerNodeRecovered())

	got := formerRootSentences(caseFacts{p: p, members: cartFailing()})

	assert.Empty(t, got)
}

func TestFormerRootSentencesNeedAFormerRoot(t *testing.T) {
	p := formerIncident(t, inventory.EntityID{}, formerNodeRecovered())
	p.Pending.MarkReplaced()

	got := formerRootSentences(caseFacts{p: p, members: cartFailing()})

	assert.Empty(t, got)
}

func TestFormerRootSentencesStaySilentWhileFormerRootFails(t *testing.T) {
	p := formerIncident(t, movedNode, formerNodeRecovered())
	p.Pending.MarkReplaced()
	members := append(cartFailing(), detection.Finding{Entity: movedNode,
		Reason: "NodeNotReady", Severity: detection.Critical,
		Since: formerAt, Summary: "Node is NotReady"})

	got := formerRootSentences(caseFacts{p: p, members: members})

	assert.Empty(t, got)
}

func TestFormerRootSentencesIgnoreAdvisoryMemberOfFormerRoot(t *testing.T) {
	p := formerIncident(t, movedNode, formerNodeRecovered())
	p.Pending.MarkReplaced()
	members := append(cartFailing(), detection.Finding{Entity: movedNode,
		Reason: "NodeNotReady", Advisory: true, Since: formerAt})

	got := formerRootSentences(caseFacts{p: p, members: members})

	assert.Len(t, got, 1)
}

func TestFormerRootSentencesNeedTheRecoveryInTheNewEvents(t *testing.T) {
	other := movedCart
	cases := map[string]incident.Event{
		"another entity": {At: formerAt, Entity: &other,
			Text: incident.RecoveredPrefix + "Deployment is healthy"},
		"no entity": {At: formerAt,
			Text: incident.RecoveredPrefix + "Node is NotReady"},
		"not a recovery": {At: formerAt, Entity: &movedNode,
			Text: "Node is NotReady (node n1)"},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			p := formerIncident(t, movedNode, event)
			p.Pending.MarkReplaced()

			got := formerRootSentences(caseFacts{p: p,
				members: cartFailing()})

			assert.Empty(t, got)
		})
	}
	t.Run("no events", func(t *testing.T) {
		p := formerIncident(t, movedNode)
		p.Pending.MarkReplaced()

		assert.Empty(t, formerRootSentences(caseFacts{p: p}))
	})
}
