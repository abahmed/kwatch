package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// A brief peak at the ceiling is only informational, but the finding is
// there at once: it explains the workloads it slows. It becomes a
// warning once the autoscaler has stayed at its maximum for the window.
func TestHPAMaxedOutIsInformationalUntilItLasts(t *testing.T) {
	model, id := limitedHPA(1, 2)

	early := evaluate(HPA{}, model, t0.Add(time.Minute), id, nil)
	require.Len(t, early.Findings, 1)
	assert.Equal(t, reasons.HPAMaxedOut, early.Findings[0].Reason)
	assert.Equal(t, detection.Info, early.Findings[0].Severity)
	assert.Equal(t, DefaultHPAMaxedFor-time.Minute, early.RecheckAfter)

	late := evaluate(HPA{}, model, t0.Add(DefaultHPAMaxedFor), id, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, detection.Warning, late.Findings[0].Severity)
}
