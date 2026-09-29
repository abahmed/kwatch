package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newProblemsServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	h := &HealthServer{diagnostics: true, diagnosticsToken: token}
	assert.NoError(t, h.ConfigureDependencies(Dependencies{
		Problems: &fakeProblemLister{snap: []ProblemView{
			{ID: "node/n1", State: "open", Members: 2},
		}},
	}))
	ts := httptest.NewServer(newServeMux(h))
	t.Cleanup(ts.Close)
	return ts
}

func doGet(t *testing.T, url, auth string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	assert.NoError(t, err)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestProblemsEndpointServesSnapshotWithToken(t *testing.T) {
	ts := newProblemsServer(t, "tok")
	resp := doGet(t, ts.URL+"/problems", "Bearer tok")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var got []ProblemView
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Len(t, got, 1)
	assert.Equal(t, "node/n1", got[0].ID)
}

func TestProblemsEndpointRejectsMissingOrWrongToken(t *testing.T) {
	ts := newProblemsServer(t, "tok")
	for _, auth := range []string{"", "Bearer nope"} {
		resp := doGet(t, ts.URL+"/problems", auth)
		assert.Contains(t,
			[]int{http.StatusUnauthorized, http.StatusForbidden},
			resp.StatusCode)
	}
}

func TestPersistenceEndpointIsRemoved(t *testing.T) {
	ts := newProblemsServer(t, "tok")
	resp := doGet(t, ts.URL+"/persistence", "Bearer tok")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
