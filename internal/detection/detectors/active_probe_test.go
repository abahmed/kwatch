package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A dependency outside the cluster that refuses connections is named by
// its endpoint.
func TestActiveProbeNamesUnreachableDependency(t *testing.T) {
	m := newTestModel()
	db := newID(kube.KindExternalEndpoint, "", "db.example.com:5432")
	put(m, db, t0.Add(-2*time.Minute), map[string]inventory.Value{
		kube.AttrHealthy:         inventory.Bool(false),
		kube.AttrProbeError:      inventory.Text("connection refused"),
		kube.AttrFailureDuration: inventory.Number(60),
	})

	got := ActiveProbe{}.Detect(testDetectorContext(m, t0), entityOf(m, db))

	require.Len(t, got, 1)
	assert.Equal(t, "Endpoint does not accept connections", got[0].Summary)
}
