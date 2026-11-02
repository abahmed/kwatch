package dashboard

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestCoverageServesWhatKwatchWatches(t *testing.T) {
	s := newTestServer()
	s.SetCoverage(func() kube.Coverage {
		return kube.Coverage{
			Namespaces: kube.NamespaceCoverage{Watched: 2,
				WatchedNames: []string{"default", "shop"}},
			Kinds: kube.KindCoverage{Watched: 40, Synced: 39,
				Unreadable: []kube.KindGap{{Kind: "secrets",
					Reason: kube.ReasonPermissionDenied}}},
			SyncedAt: testEpoch,
		}
	})

	w := get(t, s, "/api/coverage")

	require.Equal(t, http.StatusOK, w.Code)
	var got kube.Coverage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, []string{"default", "shop"}, got.Namespaces.WatchedNames)
	assert.Equal(t, 39, got.Kinds.Synced)
	assert.Equal(t, "secrets", got.Kinds.Unreadable[0].Kind)
	assert.True(t, testEpoch.Equal(got.SyncedAt))
}

func TestCoverageIsUnavailableWithoutThePipeline(t *testing.T) {
	s := newTestServer()

	assert.Equal(t, http.StatusServiceUnavailable,
		get(t, s, "/api/coverage").Code)

	s.SetCoverage(func() kube.Coverage { return kube.Coverage{} })
	s.SetCoverage(nil)
	assert.Equal(t, http.StatusServiceUnavailable,
		get(t, s, "/api/coverage").Code)
}

func TestCoverageNeedsAuthAndGet(t *testing.T) {
	s := newTestServer()
	s.SetCoverage(func() kube.Coverage { return kube.Coverage{} })

	assert.Equal(t, http.StatusUnauthorized,
		call(t, s, http.MethodGet, "/api/coverage", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed,
		call(t, s, http.MethodPost, "/api/coverage", testToken).Code)
}
