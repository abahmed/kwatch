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

// flapAPI makes the API server probe fail three times a minute apart,
// each outage lasting down, and returns the findings after the last.
func flapAPI(t *testing.T, down time.Duration) []detection.Finding {
	t.Helper()
	m := newTestModel()
	registry := detection.NewRegistry(nil, ClusterService{})
	probe := func(at time.Time, healthy bool) detection.Evaluation {
		put(m, kube.APIServer, at, map[string]inventory.Value{
			kube.AttrHealthy:    inventory.Bool(healthy),
			kube.AttrProbeError: inventory.Text("refused"),
		})
		return registry.Evaluate(m, at, kube.APIServer)
	}
	probe(t0, true)
	var last detection.Evaluation
	for i := range 3 {
		start := t0.Add(time.Duration(i+1) * time.Minute)
		probe(start, false)
		last = probe(start.Add(down), true)
	}
	return last.Findings
}

// Three single lost probes within minutes are a warning, not a restart
// loop that pages.
func TestRecurringOutageOfSingleProbesIsAWarning(t *testing.T) {
	got := flapAPI(t, 30*time.Second)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

// Three outages that each outlast a lost probe within minutes are a
// restart loop.
func TestRecurringOutageThatLastsIsCritical(t *testing.T) {
	got := flapAPI(t, 50*time.Second)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Critical, got[0].Severity)
}
