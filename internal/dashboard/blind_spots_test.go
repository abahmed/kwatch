package dashboard

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/status"
)

// The kwatch part lists the optional parts that do not work, by name,
// except the two the page already shows from their own fields.
func TestKwatchStatusListsBlindSpots(t *testing.T) {
	s := newTestServer()
	_, body := readKwatch(t, s)
	assert.NotContains(t, body, "blindSpots")

	s.SetComponentErrors(func() map[string]string {
		return map[string]string{"rbac": "permission_denied",
			"kubelet-stats": "kubelet_unreachable",
			"license":       "license_expired",
			"pod-logs":      "optional_permission_denied",
			"coverage":      "cache_sync_pending"}
	})
	got, _ := readKwatch(t, s)
	assert.Equal(t, []BlindSpot{
		{Part: "kubelet-stats", Reason: "kubelet_unreachable"},
		{Part: "license", Reason: "license_expired"},
		{Part: "rbac", Reason: "permission_denied"}}, got.BlindSpots)

	s.SetComponentErrors(nil)
	_, body = readKwatch(t, s)
	assert.NotContains(t, body, "blindSpots")
}

// /api/status names the cluster as alerts do, "" when it is unnamed.
func TestStatusNamesTheCluster(t *testing.T) {
	s := newTestServer()
	read := func() StatusResponse {
		w := get(t, s, "/api/status")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var out StatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out
	}
	assert.Empty(t, read().Cluster)
	s.SetStatus(func() status.Report {
		return status.Report{At: testEpoch, Cluster: "shop-eu"}
	})
	assert.Equal(t, "shop-eu", read().Cluster)
}
