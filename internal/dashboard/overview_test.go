package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

func lookFirstIDs(list []OverviewIssue) []string {
	out := []string{}
	for _, i := range list {
		out = append(out, i.ID)
	}
	return out
}

func TestOverviewCountsIssuesBySeverity(t *testing.T) {
	gone := issueAt("gone", "a", incident.Page, time.Hour)
	gone.State = incident.Resolved
	got := buildOverview([]incident.Incident{gone,
		issueAt("p1", "a", incident.Page, time.Hour),
		issueAt("p2", "a", incident.Page, time.Hour),
		issueAt("n1", "a", incident.Notify, time.Hour),
		issueAt("d1", "a", incident.Digest, time.Hour),
		issueAt("s1", "a", incident.Silent, time.Hour),
	}, nil)

	assert.Equal(t, SeverityCounts{Urgent: 2, Attention: 1, Low: 1,
		Quiet: 1}, got.Counts)
}

func TestOverviewLooksFirstAtMostUrgentThenLongestRunning(t *testing.T) {
	acked := issueAt("acked", "a", incident.Page, time.Minute)
	acked.Ack = &incident.Ack{}
	items := []incident.Incident{
		issueAt("old-urgent", "a", incident.Page, 5*time.Hour),
		issueAt("new-urgent", "a", incident.Page, 2*time.Hour),
		acked,
		issueAt("newest", "b", incident.Digest, 30*time.Second),
		issueAt("older", "b", incident.Notify, 3*time.Hour),
		issueAt("quiet", "b", incident.Silent, time.Second),
	}

	got := buildOverview(items, nil)

	assert.Equal(t, []string{"old-urgent", "new-urgent", "acked",
		"older", "newest"}, lookFirstIDs(got.LookFirst))
	first := got.LookFirst[1]
	assert.Equal(t, OverviewIssue{ID: "new-urgent",
		Title: "Deployment app-new-urgent in a", Namespace: "a",
		Kind: "Deployment", Name: "app-new-urgent", Severity: "page",
		State: "open", Since: testEpoch.Add(-2 * time.Hour)}, first)
	assert.True(t, got.LookFirst[2].Acked)
}

func TestOverviewLooksFirstAtFiveAtMost(t *testing.T) {
	var items []incident.Incident
	for i := range 9 {
		items = append(items, issueAt(strconv.Itoa(i), "a",
			incident.Notify, time.Duration(i)*time.Minute))
	}

	got := buildOverview(items, nil)

	assert.Equal(t, []string{"8", "7", "6", "5", "4"},
		lookFirstIDs(got.LookFirst))
}

func TestOverviewServesFailedSendsOfTheLastHour(t *testing.T) {
	s := newTestServer(issueAt("p", "a", incident.Page, time.Hour))
	w := get(t, s, "/api/overview")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "failedSendsLastHour")

	s.SetFailedSince(func(time.Time) int { return 4 })
	w = get(t, s, "/api/overview")
	var got OverviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotNil(t, got.FailedSendsLastHour)
	assert.Equal(t, 4, *got.FailedSendsLastHour)
	assert.Equal(t, 1, got.Counts.Urgent)
	assert.NotEmpty(t, w.Header().Get("ETag"))
}

func TestOverviewIsUnavailableWithoutThePipelineAndNeedsGet(t *testing.T) {
	s := newTestServer()
	s.SetIncidents(nil)

	assert.Equal(t, http.StatusServiceUnavailable,
		get(t, s, "/api/overview").Code)
	assert.Equal(t, http.StatusMethodNotAllowed,
		call(t, s, http.MethodPost, "/api/overview", testToken).Code)
	assert.Equal(t, http.StatusUnauthorized,
		call(t, s, http.MethodGet, "/api/overview", "").Code)
}
