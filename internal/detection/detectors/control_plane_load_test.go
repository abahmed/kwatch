package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func numbers(pairs ...any) map[string]inventory.Value {
	out := map[string]inventory.Value{}
	for i := 0; i < len(pairs); i += 2 {
		switch v := pairs[i+1].(type) {
		case string:
			out[pairs[i].(string)] = inventory.Text(v)
		case float64:
			out[pairs[i].(string)] = inventory.Number(v)
		}
	}
	return out
}

// loadFindings evaluates d on the API server at t0 and again after wait.
func loadFindings(
	d detection.Detector, m *inventory.Model, id inventory.EntityID,
	wait time.Duration,
) []detection.Finding {
	registry := detection.NewRegistry(nil, d)
	registry.Evaluate(m, t0, id)
	return registry.Evaluate(m, t0.Add(wait), id).Findings
}

func TestAPIServerLoadReportsSlowWritesAfterFiveMinutes(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(
		kube.AttrWritesP99, 2400.0, kube.AttrWritesCalls, 300.0,
		kube.AttrWritesSlowest, "create pods", kube.AttrEtcdP99, 40.0))

	early := loadFindings(APIServerLoad{}, m, kube.APIServer, 4*time.Minute)
	got := loadFindings(APIServerLoad{}, m, kube.APIServer, slowSustain)

	assert.Empty(t, early)
	require.Len(t, got, 1)
	assert.Equal(t, "APIServerWritesSlow", got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Equal(t, "API server writes are slow: p99 2.4 seconds, "+
		"target 1 second; slowest: create pods", got[0].Summary)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "etcd p99 per call", Value: "40 ms"})
}

func TestAPIServerLoadNamesTheSlowWebhook(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(
		kube.AttrWritesP99, 8500.0, kube.AttrWritesSlowest, "create pods"))
	put(m, inventory.CoreID(kube.KindValidatingHook, "", "policy"), t0,
		numbers(kube.AttrWebhookP99, 8000.0,
			kube.AttrWebhookSlowest, "opa.example.com"))

	got := loadFindings(APIServerLoad{}, m, kube.APIServer, extremeSustain)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity, "extreme")
	assert.Contains(t, got[0].Summary,
		"slowest: create pods via webhook opa.example.com")
}

func TestAPIServerLoadIgnoresFastWrites(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(kube.AttrWritesP99, 180.0,
		kube.AttrReadsP99, 900.0))

	assert.Empty(t, loadFindings(APIServerLoad{}, m, kube.APIServer,
		slowSustain))
}

func TestAPIServerLoadReadsNeedAHigherLimit(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(kube.AttrReadsP99, 3000.0))
	assert.Empty(t, loadFindings(APIServerLoad{}, m, kube.APIServer,
		slowSustain))

	put(m, kube.APIServer, t0, numbers(kube.AttrReadsP99, 6000.0))
	got := loadFindings(APIServerLoad{}, m, kube.APIServer, slowSustain)
	require.Len(t, got, 1)
	assert.Equal(t, "APIServerReadsSlow", got[0].Reason)
}

func TestAPIServerLoadReportsThrottlingWithItsLevel(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(kube.AttrThrottledRate, 12.0,
		kube.AttrThrottledLevel, "workload-low",
		kube.AttrQueuedRequests, 40.0))

	got := loadFindings(APIServerLoad{}, m, kube.APIServer, throttleSustain)

	require.Len(t, got, 1)
	assert.Equal(t, "APIServerThrottling", got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, "API server is rejecting 12.0 requests a second "+
		"(priority level workload-low)", got[0].Summary)
}

func TestAPIServerLoadReportsEtcdNearItsQuota(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(
		kube.AttrEtcdDBBytes, 1.8*(1<<30),
		kube.AttrObjectsResource, "events", kube.AttrObjectsCount, 90000.0))

	got := loadFindings(APIServerLoad{}, m, kube.APIServer,
		DefaultProbeFailing)

	require.Len(t, got, 1)
	assert.Equal(t, "EtcdDatabaseLarge", got[0].Reason)
	assert.Equal(t, "etcd database is 1.8 GiB, close to its default "+
		"2 GiB limit", got[0].Summary)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "most objects", Value: "90,000 events"})
}

func TestAPIServerLoadIgnoresASmallDatabase(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(kube.AttrEtcdDBBytes, 188e6,
		kube.AttrObjectsResource, "events", kube.AttrObjectsCount, 1052.0))

	assert.Empty(t, loadFindings(APIServerLoad{}, m, kube.APIServer,
		slowSustain))
}

func TestAPIServerLoadReportsAnObjectExplosion(t *testing.T) {
	m := newTestModel()
	put(m, kube.APIServer, t0, numbers(
		kube.AttrObjectsResource, "events", kube.AttrObjectsCount, 240000.0))

	got := loadFindings(APIServerLoad{}, m, kube.APIServer,
		DefaultProbeFailing)

	require.Len(t, got, 1)
	assert.Equal(t, "StorageObjectsHigh", got[0].Reason)
	assert.Equal(t, "etcd stores 240,000 events objects", got[0].Summary)
}

// A value that dips just under the limit does not clear a finding.
func TestAPIServerLoadHoldsAFindingWhileClearlyOverTheLimit(t *testing.T) {
	m := newTestModel()
	id := kube.APIServer
	put(m, id, t0, numbers(kube.AttrWritesP99, 1500.0))
	registry := detection.NewRegistry(nil, APIServerLoad{})
	registry.Evaluate(m, t0, id)
	require.NotEmpty(t, registry.Evaluate(m, t0.Add(slowSustain), id).Findings)

	put(m, id, t0.Add(slowSustain), numbers(kube.AttrWritesP99, 800.0))
	assert.NotEmpty(t, registry.Evaluate(m,
		t0.Add(slowSustain+30*time.Second), id).Findings, "still 80%")

	put(m, id, t0.Add(slowSustain+time.Minute),
		numbers(kube.AttrWritesP99, 300.0))
	assert.Empty(t, registry.Evaluate(m,
		t0.Add(slowSustain+time.Minute), id).Findings)
}
