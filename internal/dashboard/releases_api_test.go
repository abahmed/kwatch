package dashboard

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/release"
)

// releasesServer is a test server whose release book holds records.
func releasesServer(records ...release.Record) *Server {
	s := newTestServer()
	s.sources.releases.Store(&releaseBook{
		book: release.Book{Records: records}})
	return s
}

func getReleases(t *testing.T, s *Server, query string) ReleasesResponse {
	t.Helper()
	w := call(t, s, http.MethodGet, "/api/releases"+query, testToken)
	require.Equal(t, 200, w.Code, w.Body.String())
	var out ReleasesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

func TestReleasesAPIListsAndFilters(t *testing.T) {
	now := testEpoch
	s := releasesServer(
		release.Record{ID: "a", Kind: "deployment", Namespace: "shop",
			Name: "api", At: now.Add(-time.Hour), Verdict: release.Failed,
			App: "argocd/shop", Incident: "inc-1"},
		release.Record{ID: "b", Kind: "statefulset", Namespace: "data",
			Name: "db", At: now.Add(-2 * time.Hour),
			Verdict:   release.Healthy,
			RolledOut: now.Add(-2*time.Hour + time.Minute)},
		release.Record{ID: "c", Kind: "deployment", Namespace: "shop",
			Name: "old", At: now.Add(-3 * 24 * time.Hour),
			Verdict: release.Down},
	)
	all := getReleases(t, s, "")
	assert.Equal(t, "7d", all.Since)
	assert.Equal(t, 3, all.Total)
	assert.Equal(t, 2, all.Bad)
	assert.Equal(t, []string{"data", "shop"}, all.Namespaces)
	require.Len(t, all.Releases, 3)
	assert.Equal(t, "a", all.Releases[0].ID)
	assert.Contains(t, all.Releases[0].Undo.Text, "Revert the change in Git")
	assert.Empty(t, all.Releases[0].Undo.Command)
	if assert.NotNil(t, all.Releases[1].RolloutSeconds) {
		assert.InDelta(t, 60, *all.Releases[1].RolloutSeconds, 0.1)
	}
	day := getReleases(t, s, "?since=24h&verdict=bad")
	assert.Equal(t, 2, day.Total)
	require.Len(t, day.Releases, 1)
	assert.Equal(t, "a", day.Releases[0].ID)
	ns := getReleases(t, s, "?namespace=data")
	require.Len(t, ns.Releases, 1)
	assert.Equal(t, "db", ns.Releases[0].Name)
}

func TestReleasesAPIRefusesUnknownFiltersAndWaitsForItsBook(t *testing.T) {
	s := releasesServer()
	for _, q := range []string{"?since=1y", "?verdict=great",
		"?namespace=a&namespace=b", "?since=24h&since=7d"} {
		w := call(t, s, http.MethodGet, "/api/releases"+q, testToken)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
	none := newTestServer()
	w := call(t, none, http.MethodGet, "/api/releases", testToken)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestScopeCovers(t *testing.T) {
	assert.True(t, Scope{}.covers("any"))
	assert.False(t, Scope{Excluded: []string{"kube-system"}}.
		covers("kube-system"))
	assert.True(t, Scope{Watched: []string{"shop"}}.covers("shop"))
	assert.False(t, Scope{Watched: []string{"shop"}}.covers("data"))
}

// Only a workload's newest release offers an undo command: on an older
// one it would bring back what came after it. Without change history
// (not Pro) no release serves its detail, even one a Pro run kept.
func TestReleasesAPIUndoOnNewestAndDetailOnlyWithHistory(t *testing.T) {
	now := testEpoch
	restarts := 9.0
	s := releasesServer(
		release.Record{ID: "new", Kind: "deployment", Namespace: "shop",
			Name: "api", At: now.Add(-time.Hour), Verdict: release.Healthy},
		release.Record{ID: "old", Kind: "deployment", Namespace: "shop",
			Name: "api", At: now.Add(-2 * time.Hour),
			Verdict: release.Failed,
			Detail:  &release.Detail{RestartsAfter: &restarts}},
	)
	s.opts.ChangeHistory = false
	got := getReleases(t, s, "")
	require.Len(t, got.Releases, 2)
	assert.Contains(t, got.Releases[0].Undo.Command, "rollout undo")
	assert.Empty(t, got.Releases[1].Undo.Command)
	assert.False(t, got.Releases[1].Undo.Mutating)
	assert.Contains(t, got.Releases[1].Undo.Text, "newer release")
	assert.Nil(t, got.Releases[1].Detail)

	s.opts.ChangeHistory = true
	pro := getReleases(t, s, "")
	if assert.NotNil(t, pro.Releases[1].Detail) {
		assert.Equal(t, 9.0, *pro.Releases[1].Detail.RestartsAfter)
	}
}

// The page's verdict picker offers every verdict the API filters by, so
// a link with any of them shows what it filters.
func TestReleasesPageOffersEveryVerdict(t *testing.T) {
	page, err := fs.ReadFile(web, "web/js/releases.js")
	require.NoError(t, err)
	for v := range releaseVerdicts {
		if v == "" || v == "bad" {
			continue
		}
		assert.Contains(t, string(page), "\n    "+v+": [",
			"verdict %q missing from the page's verdicts", v)
	}
	assert.Contains(t, string(page), "Object.keys(VERDICTS)")
}
