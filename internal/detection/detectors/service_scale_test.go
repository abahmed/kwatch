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

// A scale-up adds an endpoint that is not ready yet while the ready
// count stands still: the new pod has its time to start, so the wait
// runs from the added endpoint, not from the old ready count.
func TestServiceDegradedBackendsWaitsAfterAScaleUp(t *testing.T) {
	m, id := serviceWithSlice(2, 2, "80", "80")
	scaled := t0.Add(time.Hour)
	put(m, newID(kube.KindEndpointSlice, "default", "api-1"), scaled,
		map[string]inventory.Value{
			kube.AttrEndpoints:      inventory.Number(3),
			kube.AttrEndpointsReady: inventory.Number(2),
			kube.AttrEndpointPorts:  inventory.Text("80"),
		})

	early := evaluate(Service{}, m, scaled.Add(14*time.Second), id, nil)
	assert.Empty(t, early.Findings,
		"a backend added 14 seconds ago is still starting")

	got := evaluate(Service{}, m, scaled.Add(DefaultDegradedBackends),
		id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceBackendsDegraded, got[0].Reason)
}
