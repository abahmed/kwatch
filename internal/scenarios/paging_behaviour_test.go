package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/replay"
)

// judgedPaging replays a committed paging scenario and judges it against
// its label.
func judgedPaging(t *testing.T, name string) (verdict, replay.Result) {
	t.Helper()
	log, e, result := replayScenario(t, labelledDir, name)
	return judge(e, log, result), result
}

// Every paging scenario is judged right: the roots, the tier, the
// message and page budgets and the causes it must not blame.
func TestPagingScenariosAreJudgedRight(t *testing.T) {
	for _, name := range []string{"namespace-outage-with-services",
		"namespace-outage-pages-once", "two-teams-two-pages",
		"zone-nodes-lost-staggered", "node-recovers-workload-stays-down"} {
		t.Run(name, func(t *testing.T) {
			v, _ := judgedPaging(t, name)
			for _, c := range v.cases {
				assert.True(t, c.Correct, "root %s, got %s", c.Expected,
					c.Actual)
			}
			assert.True(t, v.tierMatches(), "tier %s", v.tier)
			assert.False(t, v.overBudget(), "%d messages", v.messages)
			assert.False(t, v.overPages(), "%d pages", v.pages)
			assert.Empty(t, v.blamed)
			assert.Zero(t, v.falseRecoveries)
		})
	}
}

// Workloads that have a Service are still a namespace outage: one
// message names them, not one per workload.
func TestNamespaceOutageWithServicesIsOneMessage(t *testing.T) {
	v, result := judgedPaging(t, "namespace-outage-with-services")

	assert.Equal(t, 1, v.messages, "the namespace message")
	assert.Zero(t, pagesRaised(result))
}

// An outage of user-facing workloads pages the namespace once, and the
// other page-tier members never reach the pagers on their own: their
// reminders would page for an alert nobody opened.
func TestNamespaceOutagePagesTheNamespaceOnce(t *testing.T) {
	_, result := judgedPaging(t, "namespace-outage-pages-once")

	assert.Equal(t, 1, pagesRaised(result))
	held := 0
	for _, p := range result.Incidents {
		if p.Tier == incident.Notify && p.Delivery.PageHeld() {
			held++
		}
	}
	assert.Equal(t, 5, held, "five of the six are held at notify")
	for i, d := range result.Decisions {
		if d.Reason == incident.ReasonMaterialChange &&
			d.Incident.Tier == incident.Notify {
			t.Errorf("message %d repeats an announcement for the hold", i)
		}
	}
}

// Two teams in the same moment are two outages: each is paged.
func TestTwoTeamsAreEachPaged(t *testing.T) {
	_, result := judgedPaging(t, "two-teams-two-pages")

	assert.Equal(t, 2, pagesRaised(result))
}

// A zone that loses its nodes minutes apart is one outage: the incident
// that paged for the first node takes the zone as its root. No second
// page, and no "moved" closing the first alert while the outage goes on.
func TestStaggeredZoneLossIsOneIncidentAndOnePage(t *testing.T) {
	_, result := judgedPaging(t, "zone-nodes-lost-staggered")

	assert.Equal(t, 1, pagesRaised(result))
	require.NotEmpty(t, result.Decisions)
	assert.Empty(t, decisionsOf(result, incident.Resolve, ""),
		"nothing was closed as moved")
	cause := decisionsOf(result, incident.Update, incident.ReasonCauseRevised)
	require.Len(t, cause, 1)
	assert.Equal(t, "zone", string(cause[0].Incident.Root.Kind))
	assert.Equal(t, result.Decisions[0].Incident.ID, cause[0].Incident.ID)
}

// A page that moves to a workload because the node that caused it came
// back says so: the node recovered, and the workload still fails. The
// tier stays page, as a page is never un-paged.
func TestMovedPageSaysTheOldCauseRecovered(t *testing.T) {
	v, result := judgedPaging(t, "node-recovers-workload-stays-down")

	assert.False(t, v.overPages(), "%d pages", v.pages)
	revised := decisionsOf(result, incident.Update,
		incident.ReasonCauseRevised)
	require.Len(t, revised, 1)
	assert.Equal(t, incident.Page, revised[0].Incident.Tier)
	idx := 0
	for i, d := range result.Decisions {
		if d.Reason == incident.ReasonCauseRevised {
			idx = i
		}
	}
	assert.Contains(t, notification.Text(result.Messages[idx]),
		"Node n1 has recovered")
}
