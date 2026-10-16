package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
)

func entitledNode(t *testing.T, e feature.Entitlements) detection.Detector {
	t.Helper()
	for _, d := range Entitled(e) {
		if d.Name() == "node" {
			return d
		}
	}
	require.Fail(t, "node detector is not entitled")
	return nil
}

// A drain explains NotReady away only when the drain is reported. In
// Community the drain finding is dropped, so it must not hide NotReady.
func TestCommunityNotReadyIsReportedWhileDraining(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model, node := cordonedNode(now.Add(-10*time.Minute),
		now.Add(-5*time.Minute), "False", "False")
	ctx := testDetectorContext(model, now)

	community := reasonsOf(entitledNode(t, feature.CommunityEntitlements()).
		Detect(ctx, node))
	pro := reasonsOf(entitledNode(t, feature.ProEntitlements()).
		Detect(ctx, node))
	unset := reasonsOf(entitledNode(t, nil).Detect(ctx, node))

	assert.Equal(t, []string{reasons.NodeNotReady}, community)
	assert.Equal(t, []string{reasons.NodeDraining}, pro,
		"Pro treats NotReady inside the drain envelope as expected")
	assert.Equal(t, community, unset, "nil is Community")
}

func TestCommunityHeartbeatLossIsReportedWhileDraining(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model, node := cordonedNode(now.Add(-10*time.Minute),
		now.Add(-5*time.Minute), "Unknown", "False")
	ctx := testDetectorContext(model, now)

	got := reasonsOf(entitledNode(t, feature.CommunityEntitlements()).
		Detect(ctx, node))

	assert.Contains(t, got, reasons.NodeNotReady)
	assert.NotContains(t, got, reasons.NodeDraining)
}
