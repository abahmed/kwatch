package dashboard

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/release"
)

// releasesServer is a test server whose release book holds records.
func releasesServer(records ...release.Record) *Server {
	s := newTestServer()
	s.sources.releases.Store(&releaseBook{
		book: release.Book{Records: records}})
	return s
}

func releaseOfIncident(t *testing.T, s *Server, id string) *ReleaseCause {
	t.Helper()
	w := call(t, s, http.MethodGet, "/api/incidents/"+id, testToken)
	require.Equal(t, 200, w.Code, w.Body.String())
	var out IncidentDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.Release
}

// A release stamped after the incident started is not its cause.
func TestReleaseAfterTheIncidentIsNotLinked(t *testing.T) {
	now := testEpoch
	s := releasesServer(release.Record{ID: "a", Kind: "deployment",
		Namespace: "shop", Name: "api", At: now.Add(time.Hour),
		Verdict: release.Failed, Incident: "inc-1"})
	s.SetIncidents(&fakeIncidents{items: []incident.Incident{
		testIncident("inc-1", "shop", 0)}})
	assert.Nil(t, releaseOfIncident(t, s, "inc-1"))
}

func TestScopeCovers(t *testing.T) {
	assert.True(t, Scope{}.covers("any"))
	assert.False(t, Scope{Excluded: []string{"kube-system"}}.
		covers("kube-system"))
	assert.True(t, Scope{Watched: []string{"shop"}}.covers("shop"))
	assert.False(t, Scope{Watched: []string{"shop"}}.covers("data"))
}

func TestIncidentDetailCarriesItsRelease(t *testing.T) {
	now := testEpoch
	s := releasesServer(release.Record{ID: "a", Kind: "deployment",
		Namespace: "shop", Name: "api", At: now.Add(-time.Hour),
		RolledOut: now.Add(-time.Hour + time.Minute),
		Verdict:   release.Failed, App: "argocd/shop",
		Incident: "inc-1"})
	s.SetIncidents(&fakeIncidents{items: []incident.Incident{
		testIncident("inc-1", "shop", 0)}})
	got := releaseOfIncident(t, s, "inc-1")
	require.NotNil(t, got)
	assert.Equal(t, release.Failed, got.Verdict)
	assert.True(t, got.Newest)
	assert.Contains(t, got.Undo.Text, "Revert the change in Git")
	assert.Empty(t, got.Undo.Command)
	if assert.NotNil(t, got.RolloutSeconds) {
		assert.InDelta(t, 60, *got.RolloutSeconds, 0.1)
	}
	assert.Nil(t, releaseOf(release.Book{}, "inc-1"))
}

// Only a workload's newest release offers an undo command: on an older
// one it would bring back what came after it. Without change history
// (not Pro) no release serves its detail, even one a Pro run kept.
func TestReleaseUndoOnNewestAndDetailOnlyWithHistory(t *testing.T) {
	now := testEpoch
	restarts := 9.0
	book := release.Book{Records: []release.Record{
		{ID: "new", Kind: "deployment", Namespace: "shop", Name: "api",
			At: now.Add(-time.Hour), Verdict: release.Healthy,
			Incident: "n"},
		{ID: "old", Kind: "deployment", Namespace: "shop", Name: "api",
			At: now.Add(-2 * time.Hour), Verdict: release.Failed,
			Incident: "o",
			Detail:   &release.Detail{RestartsAfter: &restarts}}}}
	newest := releaseOf(book, "n")
	assert.Contains(t, newest.Undo.Command, "rollout undo")
	old := releaseOf(book, "o")
	assert.Empty(t, old.Undo.Command)
	assert.False(t, old.Newest)
	assert.Contains(t, old.Undo.Text, "newer release")
	stripDetail(&book)
	assert.Nil(t, releaseOf(book, "o").Detail)
}

// A release that went wrong without opening an issue (a regression)
// still counts and is listed, so the Overview shows it.
func TestSummarizeReleasesKeepsRegressionsWithoutAnIssue(t *testing.T) {
	now := testEpoch
	sum := summarizeReleases(release.Book{Records: []release.Record{
		{ID: "a", Name: "a", At: now.Add(-time.Hour),
			Verdict: release.Failed, Incident: "inc-1"},
		{ID: "b", Name: "b", At: now.Add(-2 * time.Hour),
			Verdict: release.Regressed},
		{ID: "c", Name: "c", At: now.Add(-3 * time.Hour),
			Verdict: release.Healthy},
		{ID: "d", Name: "d", At: now.Add(-3 * 24 * time.Hour),
			Verdict: release.Down},
	}}, now.Add(-releaseWindow))
	assert.Equal(t, 3, sum.Total)
	assert.Equal(t, 2, sum.Bad)
	assert.Equal(t, []string{"inc-1"}, sum.BadIncidents)
	require.Len(t, sum.Wrong, 2)
	assert.Empty(t, sum.Wrong[1].Incident)
}

func TestReleasesRouteIsGone(t *testing.T) {
	s := releasesServer()
	w := call(t, s, http.MethodGet, "/api/releases", testToken)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
