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

func TestActiveProbeSaysHowADependencyFailed(t *testing.T) {
	cases := map[string]struct {
		kind string
		want string
	}{
		"timeout": {kube.FailureTimeout,
			"Endpoint did not answer within 3 seconds"},
		"refused": {kube.FailureRefused, "Endpoint refused the connection"},
		"dns":     {kube.FailureDNS, "Endpoint name does not resolve"},
		"dns_other": {kube.FailureDNSLookup,
			"Endpoint DNS lookup failed"},
		"unknown": {"", "Endpoint does not accept connections"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m := newTestModel()
			db := newID(kube.KindExternalEndpoint, "", "db.example.com:5432")
			put(m, db, t0.Add(-2*time.Minute), map[string]inventory.Value{
				kube.AttrHealthy:             inventory.Bool(false),
				kube.AttrProbeError:          inventory.Text("boom"),
				kube.AttrProbeFailureKind:    inventory.Text(c.kind),
				kube.AttrProbeTimeoutSeconds: inventory.Number(3),
				kube.AttrFailureDuration:     inventory.Number(60),
			})

			got := ActiveProbe{}.Detect(testDetectorContext(m, t0),
				entityOf(m, db))

			require.Len(t, got, 1)
			assert.Equal(t, c.want, got[0].Summary)
		})
	}
}

func kwatchNetworkEntity(failed float64) (*inventory.Model,
	inventory.EntityID) {
	m := newTestModel()
	put(m, kube.KwatchSelf, t0.Add(-5*time.Minute),
		map[string]inventory.Value{
			kube.AttrDependenciesUnreachable: inventory.Number(failed),
			kube.AttrFailureDuration:         inventory.Number(90),
		})
	return m, kube.KwatchSelf
}

func TestActiveProbeAdvisesWhenKwatchReachesNoDependency(t *testing.T) {
	m, id := kwatchNetworkEntity(5)

	got := ActiveProbe{}.Detect(testDetectorContext(m, t0), entityOf(m, id))

	require.Len(t, got, 1)
	assert.Equal(t, reasons.KwatchNetworkRestricted, got[0].Reason)
	assert.Equal(t, "kwatch could not reach any of its 5 probed "+
		"dependencies; its own network may be restricted", got[0].Summary)
}

func TestActiveProbeStaysQuietOnceKwatchReachesADependency(t *testing.T) {
	m, id := kwatchNetworkEntity(0)

	got := ActiveProbe{}.Detect(testDetectorContext(m, t0), entityOf(m, id))

	assert.Empty(t, got)
}
