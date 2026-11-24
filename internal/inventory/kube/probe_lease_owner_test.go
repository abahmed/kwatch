package kube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/inventory"
)

// holderServer serves one Lease "ops/app-leader" whose holder can change;
// an empty holder serves a released Lease (no holderIdentity).
type holderServer struct {
	mu     sync.Mutex
	holder string
}

func (s *holderServer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec := `"renewTime":"2026-01-01T00:00:00.000000Z"`
	if s.holder != "" {
		spec = fmt.Sprintf(`"holderIdentity":%q,%s`, s.holder, spec)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"apiVersion":"coordination.k8s.io/v1",`+
		`"kind":"LeaseList","metadata":{},"items":[{"metadata":`+
		`{"name":"app-leader","namespace":"ops"},"spec":{%s}}]}`, spec)
}

func holderProber(
	t *testing.T, s *holderServer, model *inventory.Model,
) *Prober {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	require.NoError(t, err)
	return NewProber(ProbeConfig{Client: client, Now: fixedNow,
		Model: model})
}

func applyAll(
	t *testing.T, model *inventory.Model, obs []inventory.Observation,
) {
	t.Helper()
	for _, o := range obs {
		_, err := model.Apply(o)
		require.NoError(t, err)
	}
}

var appLease = inventory.CoreID(KindLease, "ops", "app-leader")

// A Lease the metadata watch lists and the prober reports: when the
// prober stops reporting it (released) the Lease still exists, so the
// prober takes back only its own attributes and holder edge. The
// watch's data stays and no deletion is recorded.
func TestLeaseScanWithdrawsOnlyItsOwnDataFromAWatchedLease(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	pod := inventory.CoreID(KindPod, "ops", "app-0")
	model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: fixedNow(), Entity: pod})
	model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: ObservationSource, At: fixedNow(), Entity: appLease,
		Attributes: map[string]inventory.Value{
			"watched": inventory.Text("yes")}})
	s := &holderServer{holder: "app-0_1"}
	prober := holderProber(t, s, model)
	prober.cfg.LeasesWatched = func() bool { return true }

	applyAll(t, model, prober.leases(context.Background()))
	require.Equal(t, []inventory.EntityID{pod}, model.Related(appLease,
		inventory.References, inventory.Outgoing))

	s.holder = ""
	applyAll(t, model, prober.leases(context.Background()))

	lease, ok := model.Entity(appLease)
	require.True(t, ok, "the watched Lease still exists")
	assert.Contains(t, lease.Attributes, "watched")
	assert.NotContains(t, lease.Attributes, AttrLeaseHolder)
	assert.Empty(t, model.Related(appLease, inventory.References,
		inventory.Outgoing), "the holder edge is withdrawn")
	for _, change := range model.Changes(appLease, fixedNow().Add(-1)) {
		assert.False(t, change.Deleted, "no deletion is recorded")
	}
}

// Without a watch of Leases the prober is their only source: a Lease it
// stops reporting leaves the model.
func TestLeaseScanRetiresAnUnwatchedSkippedLease(t *testing.T) {
	s := &holderServer{holder: "app-0_1"}
	prober := holderProber(t, s, inventory.NewModel(inventory.Options{}))
	prober.leases(context.Background())

	s.holder = ""
	got := leaseNames(prober.leases(context.Background()))

	assert.Equal(t, map[string]string{"app-leader": "gone"}, got)
}

// A hand-over to a holder whose pod is unknown drops the edge to the
// former holder.
func TestLeaseScanDropsTheFormerHolderEdge(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	pod := inventory.CoreID(KindPod, "ops", "app-0")
	model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: fixedNow(), Entity: pod})
	s := &holderServer{holder: "app-0_1"}
	prober := holderProber(t, s, model)
	applyAll(t, model, prober.leases(context.Background()))
	require.NotEmpty(t, model.Related(appLease, inventory.References,
		inventory.Outgoing))

	s.holder = "elsewhere_2"
	applyAll(t, model, prober.leases(context.Background()))

	assert.Empty(t, model.Related(appLease, inventory.References,
		inventory.Outgoing))
}
