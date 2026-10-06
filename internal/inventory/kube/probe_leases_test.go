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

// leaseServer serves Leases in pages of two, like an API server that
// honours the list limit. Names can be removed between scans.
type leaseServer struct {
	mu    sync.Mutex
	names []string
	limit string
	// failPage makes the request for that continue token fail.
	failPage string
}

func (s *leaseServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := r.URL.Query().Get("continue")
	if token != "" && token == s.failPage {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	s.limit = r.URL.Query().Get("limit")
	start := 0
	if token != "" {
		_, _ = fmt.Sscanf(token, "from-%d", &start)
	}
	end := min(start+2, len(s.names))
	next := ""
	if end < len(s.names) {
		next = fmt.Sprintf("from-%d", end)
	}
	items := ""
	for i, name := range s.names[start:end] {
		if i > 0 {
			items += ","
		}
		items += fmt.Sprintf(`{"metadata":{"name":%q,"namespace":"ops"},`+
			`"spec":{"holderIdentity":"h","renewTime":`+
			`"2026-01-01T00:00:00.000000Z"}}`, name)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"apiVersion":"coordination.k8s.io/v1",`+
		`"kind":"LeaseList","metadata":{"continue":%q},"items":[%s]}`,
		next, items)
}

func leaseProber(t *testing.T, s *leaseServer) *Prober {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	require.NoError(t, err)
	return NewProber(ProbeConfig{Client: client, Now: fixedNow})
}

func leaseNames(observations []inventory.Observation) map[string]string {
	out := map[string]string{}
	for _, o := range observations {
		if o.Entity.Kind == KindLease {
			out[o.Entity.Name] = o.Kind.String()
		}
	}
	return out
}

func TestLeaseScanFollowsEveryPage(t *testing.T) {
	s := &leaseServer{names: []string{"a", "b", "c", "d", "e"}}
	prober := leaseProber(t, s)

	got := leaseNames(prober.leases(context.Background()))

	assert.Len(t, got, 5, "all pages are read, not only the first")
	assert.NotEmpty(t, s.limit, "pages are requested with a limit")
}

func TestLeaseScanRetiresDeletedLeases(t *testing.T) {
	s := &leaseServer{names: []string{"a", "b", "c"}}
	prober := leaseProber(t, s)
	prober.leases(context.Background())

	s.names = []string{"a", "c"}
	got := leaseNames(prober.leases(context.Background()))

	assert.Equal(t, "gone", got["b"])
	assert.Equal(t, "observed", got["a"])
}

func TestLeaseScanKeepsLeasesWhenAPageFails(t *testing.T) {
	s := &leaseServer{names: []string{"a", "b", "c", "d"}}
	prober := leaseProber(t, s)
	prober.leases(context.Background())

	s.failPage = "from-2"
	got := leaseNames(prober.leases(context.Background()))

	assert.NotContains(t, got, "c", "an incomplete list proves nothing")
	assert.NotContains(t, got, "d")
	for _, state := range got {
		assert.NotEqual(t, "gone", state)
	}
}
