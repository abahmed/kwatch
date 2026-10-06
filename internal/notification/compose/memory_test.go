package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// oomKill is an incident of one OOM-killed container whose finding
// carries the given evidence.
func oomKill(evidence ...detection.Evidence) incident.Incident {
	id := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	return incident.Incident{
		ID: "oom-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.OOMKilled, Severity: detection.Critical,
			Since:    writerNow.Add(-time.Hour),
			Summary:  "Container is killed for exceeding its memory limit",
			Evidence: evidence}),
	}
}

func note(t *testing.T, evidence ...detection.Evidence) string {
	t.Helper()
	return Writer{}.Write(announce(oomKill(evidence...)), writerNow).Note
}

func TestWriteOOMSaysTheLimitAndThePeak(t *testing.T) {
	got := note(t,
		detection.Evidence{Label: detection.EvidenceMemoryLimit,
			Value: "512Mi"},
		detection.Evidence{Label: detection.EvidenceMemoryPeak,
			Value: "610Mi"})

	assert.Contains(t, got, "It was killed at its 512Mi memory limit; "+
		"it used 610Mi at peak in the last 24 hours.")
}

func TestWriteOOMSaysAClimbBeforeTheKill(t *testing.T) {
	got := note(t,
		detection.Evidence{Label: detection.EvidenceMemoryPeak,
			Value: "512Mi"},
		detection.Evidence{Label: detection.EvidenceMemoryRise,
			Value: "200Mi to 512Mi over 3h"})

	assert.Contains(t, got, "Its memory rose steadily from 200Mi to "+
		"512Mi over three hours before the kill.")
	assert.NotContains(t, got, "at peak")
}

func TestWriteOOMSaysAQuickKill(t *testing.T) {
	got := note(t,
		detection.Evidence{Label: detection.EvidenceMemoryLimit,
			Value: "256Mi"},
		detection.Evidence{Label: detection.EvidenceKilledAfter,
			Value: "30 seconds"})

	assert.Contains(t, got, "It hit its 256Mi memory limit within "+
		"30 seconds of starting.")
}

func TestWriteOOMWithoutUsageSaysNothingExtra(t *testing.T) {
	got := note(t)

	assert.NotContains(t, got, "at peak")
	assert.NotContains(t, got, "rose steadily")
	assert.NotContains(t, got, "of starting")
}

func TestWriteOOMAdvisesAboutTheLimitWhenUseIsKnown(t *testing.T) {
	known := oomKill(detection.Evidence{
		Label: detection.EvidenceMemoryPeak, Value: "610Mi"})
	unknown := oomKill()

	withUse := Writer{}.Write(announce(known), writerNow).Steps
	without := Writer{}.Write(announce(unknown), writerNow).Steps

	require.NotEmpty(t, withUse)
	last := withUse[len(withUse)-1]
	assert.True(t, strings.HasPrefix(last.Text, "Raise the memory limit"),
		last.Text)
	assert.Empty(t, last.Command)
	for _, step := range without {
		assert.NotContains(t, step.Text, "Raise the memory limit")
	}
}
