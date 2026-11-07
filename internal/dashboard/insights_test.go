package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// issueAt is an active incident in namespace that started ago before
// testEpoch, at tier.
func issueAt(id, namespace string, tier incident.Tier,
	ago time.Duration) incident.Incident {
	p := testIncident(id, namespace, -ago)
	p.Tier = tier
	return p
}

func insightsOf(items ...incident.Incident) InsightsResponse {
	return buildInsights(insightInput{items: items, now: testEpoch})
}

func issueIDs(list []InsightIssue) []string {
	out := []string{}
	for _, i := range list {
		out = append(out, i.ID)
	}
	return out
}

func TestInsightsTodayCountsNewResolvedAndUrgent(t *testing.T) {
	gone := issueAt("gone", "a", incident.Notify, 3*time.Hour)
	gone.State, gone.Resolved = incident.Resolved, testEpoch.Add(-time.Hour)
	old := issueAt("old", "a", incident.Page, 30*time.Hour)
	old.History = []incident.Occurrence{{Opened: testEpoch.Add(-5 *
		time.Hour), Resolved: testEpoch.Add(-4 * time.Hour)}}
	moved := issueAt("moved", "a", incident.Notify, 30*time.Hour)
	moved.State, moved.Resolved = incident.Resolved, testEpoch
	moved.SupersededBy = "x"
	quiet := issueAt("quiet", "a", incident.Silent, time.Hour)

	got := insightsOf(gone, old, moved, quiet,
		issueAt("now", "b", incident.Page, time.Minute))

	assert.Equal(t, InsightToday{New: 2, Resolved: 2, Urgent: 2}, got.Today)
}

func TestInsightsTodayFollowsTheZone(t *testing.T) {
	// testEpoch is 12:00 UTC: at UTC-13 it is 23:00 the day before, so
	// an issue from 11 hours ago is today there but not in UTC.
	p := issueAt("p", "a", incident.Notify, 13*time.Hour)

	utc := buildInsights(insightInput{items: []incident.Incident{p},
		now: testEpoch})
	west := buildInsights(insightInput{items: []incident.Incident{p},
		now: testEpoch, zone: offsetZone(-13 * 60)})

	assert.Equal(t, 0, utc.Today.New)
	assert.Equal(t, 1, west.Today.New)
}

func TestInsightsUnackedUrgentIsNewestFirstAndBounded(t *testing.T) {
	var items []incident.Incident
	for i := range 8 {
		items = append(items, issueAt("p"+strconv.Itoa(i), "a",
			incident.Page, time.Duration(i+1)*time.Hour))
	}
	acked := issueAt("acked", "a", incident.Page, 100*time.Hour)
	acked.Ack = &incident.Ack{On: acked.Root, Since: testEpoch}
	items = append(items, acked,
		issueAt("quiet", "a", incident.Silent, 200*time.Hour))

	got := insightsOf(items...)

	assert.Equal(t, 8, got.Unacked.Count)
	assert.Equal(t, []string{"p0", "p1", "p2", "p3", "p4"},
		issueIDs(got.Unacked.Issues))
	assert.Nil(t, got.Quiet)
}

func TestInsightsAfterChangeKeepsChangesWithinHalfAnHour(t *testing.T) {
	near := issueAt("near", "a", incident.Notify, time.Hour)
	near.Journal.Changes = []inventory.Change{
		insightImageChange(near.Opened.Add(-40*time.Minute), "api:1",
			"api:0"),
		insightImageChange(near.Opened.Add(-10*time.Minute),
			"reg.io/team/api:1", "reg.io/team/api:2")}
	far := issueAt("far", "a", incident.Notify, 2*time.Hour)
	far.Journal.Changes = []inventory.Change{insightImageChange(
		far.Opened.Add(-time.Hour), "api:1", "api:2")}

	got := insightsOf(near, far).AfterChange

	require.Equal(t, 1, got.Count)
	assert.Equal(t, "near", got.Issues[0].ID)
	assert.Equal(t, "image api:1 → api:2", got.Issues[0].Change)
	assert.Equal(t, "deployment/api", got.Issues[0].Object)
}

func insightImageChange(at time.Time, before, after string) inventory.Change {
	return inventory.Change{At: at,
		Entity: inventory.CoreID(kube.KindDeployment, "a", "api"),
		Fields: []inventory.FieldChange{{Path: "containers[api].image",
			Before: before, After: after}}}
}

func TestInsightsUndeliveredReadsTheLatestSendPerProvider(t *testing.T) {
	trail := map[string][]DeliveryAttempt{
		"fixed": {
			{At: testEpoch.Add(-2 * time.Hour), Channel: "slack",
				Kind: "announce", Result: "retrying", Error: "timeout"},
			{At: testEpoch.Add(-time.Hour), Channel: "slack",
				Kind: "announce", Result: "ok"}},
		"broken": {
			{At: testEpoch.Add(-time.Hour), Channel: "pagerduty",
				Kind: "announce", Result: "failed",
				Error: "401 token=abcdefghijklmnop1234"},
			{At: testEpoch.Add(-48 * time.Hour), Channel: "slack",
				Kind: "update", Result: "failed"},
			{At: testEpoch.Add(-time.Minute), Kind: "digest",
				Result: "carried"}},
	}
	got := buildInsights(insightInput{now: testEpoch,
		items: []incident.Incident{
			issueAt("fixed", "a", incident.Page, time.Hour),
			issueAt("broken", "a", incident.Page, time.Hour)},
		trail: func(id string) []DeliveryAttempt { return trail[id] },
	}).Undelivered

	require.Equal(t, 1, got.Count)
	a := got.Attempts[0]
	assert.Equal(t, "broken", a.ID)
	assert.Equal(t, "pagerduty", a.Provider)
	assert.Equal(t, "failed", a.Result)
	assert.NotContains(t, a.Error, "abcdefghijklmnop1234")
}

func TestInsightsCausesGroupByFamilyAsTheListShowsThem(t *testing.T) {
	var items []incident.Incident
	add := func(id string, mode detection.Mode, tier incident.Tier,
		resolved bool) {
		p := issueAt(id, "a", tier, time.Hour)
		p.Mode = mode
		if resolved {
			p.State = incident.Resolved
		}
		items = append(items, p)
	}
	add("1", "ImagePull.Registry", incident.Notify, false)
	add("2", "ImagePull", incident.Notify, true)
	add("3", "ImagePull", incident.Silent, false)
	add("4", "OOMKilled.Limit", incident.Page, false)
	add("5", "LeaseStale", incident.Notify, false)
	add("6", "", incident.Notify, false)

	got := insightsOf(items...).Causes

	assert.Equal(t, []InsightCause{
		{Mode: "ImagePull", Label: "Image pull failures", Count: 2,
			Active: 1},
		{Mode: "LeaseStale", Label: "Lease stale", Count: 1, Active: 1},
		{Mode: "OOMKilled", Label: "Out of memory", Count: 1,
			Active: 1}}, got)
}

func TestInsightsNamespaceMapOrdersBySeverityAndIsBounded(t *testing.T) {
	items := []incident.Incident{
		issueAt("a1", "low", incident.Digest, time.Hour),
		issueAt("a2", "low", incident.Digest, time.Hour),
		issueAt("a3", "low", incident.Digest, time.Hour),
		issueAt("b1", "hot", incident.Page, time.Hour),
		issueAt("b2", "hot", incident.Digest, time.Hour),
		issueAt("c1", "warm", incident.Notify, time.Hour),
		issueAt("q1", "quiet", incident.Silent, time.Hour),
		issueAt("cl", "", incident.Page, time.Hour),
	}
	got := insightsOf(items...).Namespaces
	assert.Equal(t, []NamespaceTile{
		{Namespace: "hot", Severity: "page", Count: 2, Urgent: 1,
			Low: 1},
		{Namespace: "warm", Severity: "notify", Count: 1, Attention: 1},
		{Namespace: "low", Severity: "digest", Count: 3, Low: 3}},
		got.Tiles)
	assert.Zero(t, got.More)

	items = nil
	for i := range maxInsightTiles + 5 {
		items = append(items, issueAt(strconv.Itoa(i),
			"ns-"+strconv.Itoa(i), incident.Notify, time.Hour))
	}
	got = insightsOf(items...).Namespaces
	assert.Len(t, got.Tiles, maxInsightTiles)
	assert.Equal(t, 5, got.More)
}

func TestInsightsNodesCountIssuesOnEachNode(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	n1 := inventory.CoreID(kube.KindNode, "", "n1")
	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	tm.observe(n1, nil)
	tm.observe(n2, nil)
	tm.relate(pod, inventory.RunsOn, n1)
	p := issueAt("p", "shop", incident.Page, time.Hour)
	p.Root = root
	nodeIssue := issueAt("n", "", incident.Notify, time.Hour)
	nodeIssue.Root = n1
	other := issueAt("o", "", incident.Notify, time.Hour)
	other.Root = n2

	got := buildInsights(insightInput{now: testEpoch, model: tm.m,
		items: []incident.Incident{p, nodeIssue, other}}).Nodes

	assert.Equal(t, []InsightNode{{Name: "n1", Issues: 2, Urgent: 1},
		{Name: "n2", Issues: 1}}, got)
	assert.Empty(t, insightsOf(p).Nodes)
}

func TestOverviewTagsTheBodyAndValidatesTZ(t *testing.T) {
	s := newTestServer(issueAt("p", "a", incident.Page, time.Hour))

	w := getPart(t, s, "/api/overview?tz=60", "insights")
	require.Equal(t, http.StatusOK, w.Code)
	var got InsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 1, got.Unacked.Count)
	tag := w.Header().Get("ETag")
	require.NotEmpty(t, tag)

	again := getIfNoneMatch(t, s, "/api/overview?tz=60", tag)
	assert.Equal(t, http.StatusNotModified, again.Code)

	assert.Equal(t, http.StatusBadRequest,
		get(t, s, "/api/overview?tz=9999").Code)
	assert.Equal(t, http.StatusMethodNotAllowed,
		call(t, s, http.MethodPost, "/api/overview", testToken).Code)
	assert.Equal(t, http.StatusUnauthorized,
		call(t, s, http.MethodGet, "/api/overview", "").Code)
	s.SetIncidents(nil)
	assert.Equal(t, http.StatusServiceUnavailable,
		get(t, s, "/api/overview").Code)
}

func TestInsightsExplainAQuietCluster(t *testing.T) {
	tm := newTestModel(t)
	tm.deployment()
	s := newTestServer(issueAt("q", "a", incident.Silent, time.Hour))
	s.SetModel(tm.m)
	s.SetCoverage(func() kube.Coverage {
		return kube.Coverage{
			Namespaces: kube.NamespaceCoverage{Watched: 34},
			Kinds:      kube.KindCoverage{Watched: 40}}
	})
	s.NoteScan()

	w := getPart(t, s, "/api/overview", "insights")
	require.Equal(t, http.StatusOK, w.Code)
	var got InsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	require.NotNil(t, got.Quiet)
	require.NotNil(t, got.Quiet.Objects)
	assert.Equal(t, 2, *got.Quiet.Objects)
	assert.Equal(t, 34, *got.Quiet.Namespaces)
	assert.Equal(t, 40, *got.Quiet.Kinds)
	assert.True(t, testEpoch.Equal(got.Quiet.LastScan))
}
