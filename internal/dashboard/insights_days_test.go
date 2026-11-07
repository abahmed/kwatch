package dashboard

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
)

// dayCounts is "MM-DD=opened/resolved/paged" per day.
func dayCounts(days []InsightDay) []string {
	out := []string{}
	for _, d := range days {
		out = append(out, fmt.Sprintf("%s=%d/%d/%d", d.Date[5:], d.Opened,
			d.Resolved, d.Paged))
	}
	return out
}

func TestInsightsDaysCountOpenedResolvedAndPaged(t *testing.T) {
	// trendNow is Wednesday 28 Oct, 12:00 UTC.
	urgent := opened("u", "a", incident.Page, ago(30*time.Hour),
		ago(2*time.Hour))
	urgent.History = []incident.Occurrence{{Opened: ago(30 * time.Hour),
		Resolved: ago(29 * time.Hour), Page: false}}
	notify := opened("n", "a", incident.Notify, ago(50*time.Hour))
	notify.State, notify.Resolved = incident.Resolved, ago(time.Hour)
	moved := opened("m", "a", incident.Notify, ago(3*time.Hour))
	moved.State, moved.Resolved = incident.Resolved, ago(time.Hour)
	moved.SupersededBy = "x"
	quiet := opened("q", "a", incident.Silent, ago(time.Hour))

	got := insightDays([]incident.Incident{urgent, notify, moved, quiet},
		trendNow, time.UTC)

	assert.Equal(t, []string{"10-22=0/0/0", "10-23=0/0/0",
		"10-24=0/0/0", "10-25=0/0/0", "10-26=1/0/0", "10-27=1/1/0",
		"10-28=2/1/1"}, dayCounts(got))
}

func TestInsightsDaysPagedForAnEarlierUrgentOccurrence(t *testing.T) {
	p := opened("p", "a", incident.Notify, ago(26*time.Hour),
		ago(time.Hour))
	p.History = []incident.Occurrence{{Opened: ago(26 * time.Hour),
		Resolved: ago(25 * time.Hour), Page: true}}

	got := insightDays([]incident.Incident{p}, trendNow, time.UTC)

	assert.Equal(t, "10-27=1/1/1", dayCounts(got)[5])
	assert.Equal(t, "10-28=1/0/0", dayCounts(got)[6])
}

// A week across the end of summer time counts each day in the zone's
// own offset that day, and the 25-hour day is still one day.
func TestInsightsDaysFollowTheZoneNameAcrossADaylightSavingChange(
	t *testing.T,
) {
	// Berlin leaves summer time on 25 Oct 2026. 22:30 UTC on 22 Oct is
	// 00:30 on 23 Oct in Berlin (UTC+2) but 23:30 on 22 Oct at UTC+1.
	early := time.Date(2026, 10, 22, 22, 30, 0, 0, time.UTC)
	// 23:30 UTC on 25 Oct is 00:30 on 26 Oct in Berlin (UTC+1 by then).
	late := time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)
	items := []incident.Incident{
		opened("a", "shop", incident.Page, early),
		opened("b", "shop", incident.Notify, late),
	}
	s := newTestServer(items...)
	s.now = clock.Func(func() time.Time { return trendNow })

	w := getPart(t, s,
		"/api/overview?tzName=Europe/Berlin&tz=60", "insights")
	require.Equal(t, 200, w.Code, w.Body.String())
	var got InsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	assert.Equal(t, []string{"10-22=0/0/0", "10-23=1/0/1",
		"10-24=0/0/0", "10-25=0/0/0", "10-26=1/0/0", "10-27=0/0/0",
		"10-28=0/0/0"}, dayCounts(got.Days))

	fixed := insightDays(items, trendNow, offsetZone(60))
	assert.Equal(t, "10-22=1/0/1", dayCounts(fixed)[0])
}

func TestInsightsAfterChangeCountsOutOfTheActiveIssues(t *testing.T) {
	got := insightsOf(issueAt("a", "x", incident.Notify, time.Hour),
		issueAt("b", "x", incident.Page, time.Hour))

	assert.Equal(t, 0, got.AfterChange.Count)
	assert.Equal(t, 2, got.AfterChange.Total)
	assert.Len(t, got.Days, trendDays)
}

// Each day carries its untold openings, as the overview's trends count them, so
// the Insights page needs no second request for them.
func TestInsightsDaysCountUntoldOpeningsAsTrendsDo(t *testing.T) {
	quiet := untoldPage("q", ago(2*time.Hour))
	told := opened("t", "shop", incident.Page, ago(3*time.Hour))
	unheard := opened("u", "shop", incident.Page, ago(5*time.Hour),
		ago(4*time.Hour))
	unheard.History = []incident.Occurrence{{Opened: ago(5 * time.Hour),
		Resolved: ago(5*time.Hour - time.Minute), Heard: false}}
	items := []incident.Incident{quiet, told, unheard}

	days := insightDays(items, trendNow, time.UTC)
	trends := buildTrends(items, trendNow, time.UTC, 0)

	for i, d := range days {
		assert.Equal(t, trends.Days[i].Untold, d.Untold, d.Date)
	}
	today := days[len(days)-1]
	assert.Equal(t, 2, today.Untold)
	assert.Equal(t, 4, today.Opened, "untold openings are openings")
	body, err := json.Marshal(InsightDay{})
	require.NoError(t, err)
	assert.Contains(t, string(body), `"untold":0`)
}
