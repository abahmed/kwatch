package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Error shares read from the servers' own metrics are reported once
// sustained, with a floor of traffic.
func TestClusterServiceReportsErrorShares(t *testing.T) {
	m := newTestModel()
	since := t0.Add(-10 * time.Minute)
	put(m, kube.APIServer, since, map[string]inventory.Value{
		kube.AttrHealthy:        inventory.Bool(true),
		kube.AttrLatencyMS:      inventory.Number(20),
		kube.AttrAPIRequestRate: inventory.Number(50),
		kube.AttrAPIErrorRate:   inventory.Number(5),
	})
	put(m, kube.ClusterDNS, since, map[string]inventory.Value{
		kube.AttrHealthy:         inventory.Bool(true),
		kube.AttrDNSRequestRate:  inventory.Number(0.2),
		kube.AttrDNSServfailRate: inventory.Number(0.1),
	})

	api := ClusterService{}.Detect(testDetectorContext(m, t0),
		entityOf(m, kube.APIServer))
	dns := ClusterService{}.Detect(testDetectorContext(m, t0),
		entityOf(m, kube.ClusterDNS))

	require.Len(t, api, 1)
	assert.Equal(t, reasons.APIServerErrors, api[0].Reason)
	assert.Contains(t, api[0].Summary, "10% of requests")
	assert.Empty(t, dns, "too little traffic for a share")
}
