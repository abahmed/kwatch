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

// A run that ended in Error long before kwatch first saw it is dated by
// its own finish time, and says what it stated as the reason.
func TestFailedRunIsDatedByItsFinishAndSaysWhy(t *testing.T) {
	finished := t0.Add(-18 * 30 * 24 * time.Hour)
	m, id := genericEntity(map[string]inventory.Value{
		kube.AttrPhase:    inventory.Text("Error"),
		kube.AttrFinished: inventory.Time(finished),
		kube.AttrMessage:  inventory.Text("child 'nightly-2' failed"),
	})

	got := evaluate(Generic{}, m, t0.Add(time.Minute), id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.PhaseFailed, got[0].Reason)
	assert.True(t, finished.Equal(got[0].Since), got[0].Since.String())
	assert.Equal(t, "Reports phase Error: child 'nightly-2' failed",
		got[0].Summary)
}
