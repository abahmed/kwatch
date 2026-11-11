package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// untoldPage is a page-tier incident that settled before alerting: no
// message of it went out.
func untoldPage(id string, at time.Time) incident.Incident {
	p := opened(id, "shop", incident.Page, at)
	p.Announced = time.Time{}
	p.State, p.Resolved = incident.Resolved, at.Add(time.Minute)
	p.Problem = "api in shop keeps crashing."
	return p
}

// An incident that settled before alerting is no page in the charts,
// and its earlier, unheard occurrences are none either.
func TestTrendsCountUntoldOpeningsAsNoPage(t *testing.T) {
	quiet := untoldPage("q", ago(2*time.Hour))
	told := opened("t", "shop", incident.Page, ago(3*time.Hour))
	unheard := opened("u", "shop", incident.Page, ago(5*time.Hour),
		ago(4*time.Hour))
	unheard.History = []incident.Occurrence{{Opened: ago(5 * time.Hour),
		Resolved: ago(5*time.Hour - time.Minute), Heard: false}}

	got := buildTrends([]incident.Incident{quiet, told, unheard}, trendNow,
		time.UTC, 0)

	today := got.Days[len(got.Days)-1]
	assert.Equal(t, 2, today.Page, "the told openings")
	assert.Equal(t, 2, today.Untold)
	assert.Equal(t, 4, today.Total)

	days := insightDays([]incident.Incident{quiet, told}, trendNow, time.UTC)
	assert.Equal(t, 1, days[len(days)-1].Paged)
}

// The list names an untold incident by its problem, never its key,
// and marks it so the page can say it settled before alerting.
func TestListItemOfUntoldIncidentHasProblemTitleAndMark(t *testing.T) {
	got := toListItem(untoldPage("q", ago(time.Hour)))

	assert.Equal(t, "api in shop keeps crashing.", got.Title)
	assert.True(t, got.Untold)
	assert.False(t, toListItem(opened("t", "shop", incident.Page,
		ago(time.Hour))).Untold)
}

// A title is the problem, not the headline of a later message such as
// a resolve; the root in words is only the last resort.
func TestTitlePrefersTheProblemOverTheLastHeadline(t *testing.T) {
	p := opened("t", "shop", incident.Notify, ago(time.Hour))
	p.Headline = "api in shop is healthy again."
	p.Problem = "api in shop keeps crashing."
	assert.Equal(t, p.Problem, toListItem(p).Title)

	p.Problem, p.Headline = "", ""
	assert.Equal(t, rootWords(p.Root), toListItem(p).Title)
}

// The last-resort title is words, never the raw key: the Kind as the
// API spells it, and no namespace for a cluster-scoped object.
func TestTitleForNamesTheRootInWords(t *testing.T) {
	p := opened("t", "staging", incident.Notify, ago(time.Hour))
	p.Problem, p.Headline = "", ""
	p.Root = inventory.EntityID{Kind: "service", Namespace: "staging",
		Name: "surface"}
	assert.Equal(t, "Service surface in staging", toListItem(p).Title)

	p.Root = inventory.EntityID{Kind: "mutatingwebhookconfiguration",
		Name: "vpa-webhook-config"}
	assert.Equal(t, "MutatingWebhookConfiguration vpa-webhook-config",
		toListItem(p).Title)

	p.Root = inventory.EntityID{Kind: "cluster-dns", Name: "dns"}
	assert.Equal(t, "Cluster-dns dns", toListItem(p).Title)
}

// The week's "Resolved" count and the median beside it are over the
// same issues: an issue moved into another (superseded) is in neither,
// and one that settled on its own is in neither of the alerted ones.
func TestWeekResolvedCountMatchesTheMedianCount(t *testing.T) {
	notify := opened("n", "a", incident.Notify, ago(50*time.Hour))
	notify.State, notify.Resolved = incident.Resolved, ago(time.Hour)
	moved := opened("m", "a", incident.Notify, ago(3*time.Hour))
	moved.State, moved.Resolved = incident.Resolved, ago(time.Hour)
	moved.SupersededBy = "x"
	settled := opened("s", "a", incident.Notify, ago(5*time.Hour))
	settled.State, settled.Resolved = incident.Resolved, ago(4*time.Hour)
	settled.Announced = time.Time{}
	again := opened("r", "a", incident.Page, ago(30*time.Hour),
		ago(2*time.Hour))
	again.History = []incident.Occurrence{{Opened: ago(30 * time.Hour),
		Resolved: ago(29 * time.Hour), Heard: true}}
	items := []incident.Incident{notify, moved, settled, again}

	days := insightDays(items, trendNow, time.UTC)
	tr := buildTrends(items, trendNow, time.UTC, incident.DefaultRemember)
	resolved, alerted := 0, 0
	for _, d := range days {
		resolved += d.Resolved
		alerted += d.ResolvedAlerted
	}
	if resolved != tr.Resolve.Count || alerted != tr.ResolveAlerted.Count {
		t.Errorf("resolved %d/%d alerted, medians over %d/%d",
			resolved, alerted, tr.Resolve.Count, tr.ResolveAlerted.Count)
	}
	if alerted != 2 {
		t.Errorf("alerted resolves = %d, want 2", alerted)
	}
}
