package kube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestBelowMinimumVersion(t *testing.T) {
	cases := map[string]bool{
		"v1.22.17-eks-1": true, "v1.20.0": true, "v1.23.0": false,
		"v1.31.2-gke.100": false, "v1.22+": true, "": false,
		"garbage": false, "v2.0.0": false,
	}
	for version, want := range cases {
		assert.Equal(t, want, BelowMinimumVersion(version), version)
	}
}

// The prober warns when the server is older than the supported minimum
// (warnOldServer logs once per prober).
func TestProberWarnsAboutAnOldServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"gitVersion":"v1.22.17"}`))
		}))
	defer srv.Close()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	require.NoError(t, err)
	p := NewProber(ProbeConfig{Client: client})

	attrs := map[string]inventory.Value{}
	p.addServerVersion(context.Background(), attrs)

	assert.True(t, p.versionWarned)
	assert.Equal(t, inventory.Text("v1.22.17"), attrs[AttrServerVersion])
}
