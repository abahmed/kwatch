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

// A pod the node controller marked NodeLost is reported at once: its
// node stopped reporting, and it needs no grace period to be sure.
func TestPodNodeLostIsReportedWhileTerminating(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pod := buildPod("test", "default", now.Add(-time.Minute),
		map[string]inventory.Value{
			kube.AttrPhase:    inventory.Text("Running"),
			kube.AttrDeleting: inventory.Bool(true),
			kube.AttrReason:   inventory.Text("NodeLost"),
		})

	findings := NewPod(PodThresholds{}).Detect(
		testDetectorContext(nil, now), pod)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.PodStatusUnknown, findings[0].Reason)
}
