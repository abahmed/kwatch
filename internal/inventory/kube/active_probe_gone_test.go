package kube

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func callEndpoints(
	t *testing.T, m *inventory.Model, pod inventory.EntityID, names ...string,
) {
	t.Helper()
	var targets []inventory.EntityID
	for _, name := range names {
		targets = append(targets,
			inventory.CoreID(KindExternalEndpoint, "", name))
	}
	_, err := m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: ObservationSource, At: fixedNow(),
		Entity: pod, Relation: inventory.Calls, Targets: targets,
	})
	require.NoError(t, err)
}

// A repointed app stops calling its old endpoint. The probe entity for it
// must go away, or its failure never resolves.
func TestActiveProberRetiresEndpointsNobodyCallsAnymore(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	pod := inventory.CoreID(KindPod, "shop", "api")
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: ObservationSource,
		At: fixedNow(), Entity: pod,
	})
	require.NoError(t, err)
	callEndpoints(t, model, pod, "old.example.com:5432")
	prober := NewActiveProber(ActiveProbeConfig{
		AutoDependencies: true, Model: model, Now: fixedNow,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, context.DeadlineExceeded
		},
		Submit: func(_ context.Context, obs ...inventory.Observation) {
			for _, o := range obs {
				_, err := model.Apply(o)
				require.NoError(t, err)
			}
		},
	})
	old := inventory.CoreID(KindExternalEndpoint, "", "old.example.com:5432")
	prober.round(context.Background())
	require.True(t, model.Exists(old))

	callEndpoints(t, model, pod, "new.example.com:5432")
	for i := 0; i < endpointGraceRounds-1; i++ {
		prober.round(context.Background())
		assert.True(t, model.Exists(old), "kept through the grace")
	}
	prober.round(context.Background())
	assert.False(t, model.Exists(old), "no caller left")
	assert.True(t, model.Exists(inventory.CoreID(
		KindExternalEndpoint, "", "new.example.com:5432")))
}

func TestDependencyProbeRefusesPrivateAddresses(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	prober := NewActiveProber(ActiveProbeConfig{
		AutoDependencies: true, Now: fixedNow, Timeout: time.Second,
	})
	endpoint := inventory.CoreID(KindExternalEndpoint, "",
		listener.Addr().String())

	observation := prober.checkDependency(context.Background(), endpoint)

	_, known := observation.Attributes[AttrHealthy]
	assert.False(t, known, "a refused probe is not a dependency failure")
	assert.Equal(t, "blocked: private address",
		observation.Attributes[AttrProbeError].AsText())
}

func TestPrivateAddressRanges(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1",
		"192.168.1.1", "169.254.169.254", "fd00::1", "fe80::1", "0.0.0.0",
		"100.64.0.1", "::ffff:10.0.0.1", "0.1.2.3", "192.0.0.8",
		"198.18.0.1", "198.19.255.255", "240.0.0.1", "255.255.255.255",
		"64:ff9b::a00:1"}
	for _, text := range blocked {
		assert.True(t, privateAddress(net.ParseIP(text)), text)
	}
	for _, text := range []string{"93.184.216.34", "2606:4700::1111"} {
		assert.False(t, privateAddress(net.ParseIP(text)), text)
	}
}

// An endpoint that is still called but was left out of the probe cap is
// not abandoned, and must keep its entity.
func TestActiveProberKeepsCalledEndpointsOutsideTheCap(t *testing.T) {
	prober := NewActiveProber(ActiveProbeConfig{Now: fixedNow})
	endpoint := inventory.CoreID(KindExternalEndpoint, "", "x.example.com:1")
	prober.probed = map[inventory.EntityID]int{endpoint: 0}
	called := map[inventory.EntityID]bool{endpoint: true}
	for range 2 * endpointGraceRounds {
		assert.Empty(t, prober.retire(nil, called), "still called")
	}
	var gone []inventory.Observation
	for range endpointGraceRounds {
		gone = append(gone, prober.retire(nil, nil)...)
	}
	assert.Len(t, gone, 1, "retired once nobody calls it")
}
