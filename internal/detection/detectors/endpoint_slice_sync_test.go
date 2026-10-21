package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withoutEndpointSlices is a sync function for a cluster whose
// EndpointSlices are not watched yet: every other kind is synced.
func withoutEndpointSlices(kind inventory.Kind) bool {
	return kind != kube.KindEndpointSlice
}

// A Service that selects nothing behind an exposure is only a failure
// when its EndpointSlices say it has no endpoints. Until that kind is
// synced an empty list means "not seen yet", so nothing is said.
func TestServiceNoEndpointsWaitsForEndpointSliceSync(t *testing.T) {
	m, id := emptyService("app=apii", true)
	now := t0.Add(time.Hour)
	require.Len(t, evaluate(Service{}, m, now, id, nil).Findings, 1,
		"the same Service is a finding once the slices are known")

	got := evaluate(Service{}, m, now, id, withoutEndpointSlices)

	assert.Empty(t, got.Findings)
	assert.Contains(t, got.Unsynced, kube.KindEndpointSlice)
}

// A Service with only unready endpoints is not reported while the
// EndpointSlices are unsynced either.
func TestServiceNoReadyBackendsWaitsForEndpointSliceSync(t *testing.T) {
	m, id := serviceWithSlice(2, 0, "80", "80")
	now := t0.Add(time.Hour)
	evaluate(Service{}, m, now, id, nil)
	require.Len(t, evaluate(Service{}, m, now.Add(DefaultNoEndpoints), id,
		nil).Findings, 1)

	got := evaluate(Service{}, m, now.Add(DefaultNoEndpoints), id,
		withoutEndpointSlices)

	assert.Empty(t, got.Findings)
	assert.Contains(t, got.Unsynced, kube.KindEndpointSlice)
}

// A webhook whose Service has no ready endpoints is reported only when
// the slices are synced: readyEndpoints says "unknown", not "zero".
func TestWebhookNoReadyEndpointsWaitsForEndpointSliceSync(t *testing.T) {
	m, id := webhook("Fail", true, []float64{0, 0})
	require.Len(t, evaluate(Webhook{}, m, t0, id, nil).Findings, 1)

	got := evaluate(Webhook{}, m, t0, id, withoutEndpointSlices)

	assert.Empty(t, got.Findings)
	assert.Contains(t, got.Unsynced, kube.KindEndpointSlice)
}

// An init container waiting for a Service may still be stuck, but while
// the EndpointSlices are unsynced the summary does not claim the Service
// has no ready endpoints.
func TestInitWaitSaysNothingOfEndpointsBeforeTheSlicesSync(t *testing.T) {
	m, c := initModel("shop/db:5432")
	svc := newID(kube.KindService, "shop", "db")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=db"),
	})
	slice := newID(kube.KindEndpointSlice, "shop", "db-1")
	put(m, slice, t0, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(1),
		kube.AttrEndpointsReady: inventory.Number(0),
	})
	link(m, slice, inventory.Backs, svc)
	now := t0.Add(6 * time.Minute)
	known := evaluate(InitWait{}, m, now, c, nil).Findings
	require.Len(t, known, 1)
	require.Contains(t, known[0].Summary, "has no ready endpoints")

	got := evaluate(InitWait{}, m, now, c, withoutEndpointSlices)

	require.Len(t, got.Findings, 1)
	assert.Equal(t, "is stuck in init: container wait-for-db has been "+
		"running for 6m", got.Findings[0].Summary)
	assert.Contains(t, got.Unsynced, kube.KindEndpointSlice)
}
