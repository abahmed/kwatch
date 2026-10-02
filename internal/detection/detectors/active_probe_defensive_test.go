package detectors

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func unhealthyProbe(duration float64, probeErr string,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kube.KindService, "ns", "web")
	attrs := map[string]inventory.Value{
		kube.AttrHealthy:         inventory.Bool(false),
		kube.AttrFailureDuration: inventory.Number(duration),
	}
	if probeErr != "" {
		attrs[kube.AttrProbeError] = inventory.Text(probeErr)
	}
	put(m, id, t0, attrs)
	return m, id
}

// TestActiveProbeFailureDurationIsBounded: a negative or huge failure
// duration must not overflow time.Duration into an instant report.
func TestActiveProbeFailureDurationIsBounded(t *testing.T) {
	for _, seconds := range []float64{math.MaxFloat64, math.Inf(1)} {
		m, id := unhealthyProbe(seconds, "refused")
		got := evaluate(ActiveProbe{}, m, t0.Add(time.Hour), id, nil)
		assert.Empty(t, got.Findings, "seconds=%v", seconds)
	}
	m, id := unhealthyProbe(-5, "refused")
	got := evaluate(ActiveProbe{}, m, t0, id, nil)
	require.Len(t, got.Findings, 1, "negative duration reports at once")
}

// TestFailureEvidenceOmitsEmptyError: an unreported probe error adds no
// empty evidence row.
func TestFailureEvidenceOmitsEmptyError(t *testing.T) {
	m, id := unhealthyProbe(0, "")
	got := evaluate(ActiveProbe{}, m, t0, id, nil)
	require.Len(t, got.Findings, 1)
	assert.Empty(t, got.Findings[0].Evidence)

	api := kube.APIServer
	put(m, api, t0, map[string]inventory.Value{
		kube.AttrHealthy: inventory.Bool(false),
	})
	got = evaluate(ClusterService{}, m, t0.Add(time.Hour), api, nil)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, detection.Critical, got.Findings[0].Severity)
	assert.Empty(t, got.Findings[0].Evidence)
}

// TestWebhookWithoutModelFindsNothing: like its siblings, the webhook
// detector tolerates a context without a model.
func TestWebhookWithoutModelFindsNothing(t *testing.T) {
	e := inventory.Entity{ID: newID(kube.KindValidatingHook, "", "hook")}
	assert.Empty(t, Webhook{}.Detect(detection.Context{}, e))
}
