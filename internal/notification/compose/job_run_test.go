package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestWriteLongJobComparesItWithRecentRuns(t *testing.T) {
	id := inventory.CoreID(kube.KindJob, "billing", "invoice-export-123")
	p := incident.Incident{
		ID: "job-1", Root: id, Tier: incident.Notify, State: incident.Open,
		Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.JobRunningLong, Severity: detection.Warning,
			Since:   writerNow.Add(-time.Hour),
			Summary: "Job has run far longer than its recent runs",
			Evidence: []detection.Evidence{
				{Label: detection.EvidenceRunningFor, Value: "2h10m"},
				{Label: detection.EvidenceRecentRuns, Value: "8–12m"},
			}}),
	}

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "Job invoice-export-123 in billing has "+
		"run far longer than its recent runs.")
	assert.Contains(t, msg.Note, "It has run 2h10m; recent runs took 8–12m.")
}
