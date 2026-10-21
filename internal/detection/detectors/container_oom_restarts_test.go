package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A container killed for memory over and over is reported from the
// third kill, not from the fifth restart: a leak that restarts it every
// quarter of an hour would otherwise stay unheard for an hour.
func TestContainerRepeatedOOMIsReportedFromThirdRestart(t *testing.T) {
	cases := []struct {
		restarts float64
		reason   string
	}{
		{2, ""},
		{3, reasons.OOMKilled},
		{4, reasons.OOMKilled},
		{5, reasons.OOMKilled},
	}
	for _, c := range cases {
		m, id := oomContainer(map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(c.restarts),
		})

		got := Container{}.Detect(testDetectorContext(m, t0),
			entityOf(m, id))

		if c.reason == "" {
			assert.Empty(t, got, "restarts %v", c.restarts)
			continue
		}
		require.Len(t, got, 1, "restarts %v", c.restarts)
		assert.Equal(t, c.reason, got[0].Reason)
		assert.Equal(t, detection.Critical, got[0].Severity)
		assert.Contains(t, got[0].Summary, "repeatedly killed")
	}
}

// Four restarts that were not memory kills stay below the threshold of
// the generic restart finding.
func TestContainerFewRestartsOtherThanOOMStayQuiet(t *testing.T) {
	m, id := oomContainer(map[string]inventory.Value{
		kube.AttrRestarts:   inventory.Number(4),
		kube.AttrLastReason: inventory.Text(reasons.Error),
	})

	got := Container{}.Detect(testDetectorContext(m, t0), entityOf(m, id))

	assert.Empty(t, got)
}
