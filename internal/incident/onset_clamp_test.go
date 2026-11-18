package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An incident whose first pods were replaced keeps its start: the
// onset is never later than when it opened, while a member that began
// failing before the opening still moves it earlier.
func TestOnsetIsNeverAfterTheOpening(t *testing.T) {
	opened := time.Date(2026, 11, 12, 5, 7, 0, 0, time.UTC)
	pod := inventory.CoreID(kube.KindPod, "staging", "tank-new")
	late := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff",
		Since: opened.Add(80 * time.Hour)}
	p := Incident{Opened: opened,
		Members: map[detection.Key]detection.Finding{late.Key(): late}}

	assert.Equal(t, opened, p.Onset(), "a replaced member")

	early := late
	early.Reason, early.Since = "Error", opened.Add(-time.Hour)
	p.Members[early.Key()] = early
	assert.Equal(t, opened.Add(-time.Hour), p.Onset())
}
