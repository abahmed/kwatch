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

func preemptedPod(name string, at time.Time) detection.Finding {
	return detection.Finding{
		Entity: inventory.CoreID(kube.KindPod, "shop", name),
		Reason: reasons.PodPreempted, Severity: detection.Warning,
		Since:   at,
		Summary: "Pod was preempted by the scheduler",
		Evidence: []detection.Evidence{
			{Label: detection.EvidencePreemptor, Value: "batch/importer-x"},
			{Label: detection.EvidencePriorities, Value: "1000 > 0"},
			{Label: detection.EvidencePreemptedAt,
				Value: at.UTC().Format(time.RFC3339)}},
	}
}

func preemptionNote(victims ...detection.Finding) string {
	return Writer{}.Write(announce(incident.Incident{
		ID: "pre-1", Root: victims[0].Entity, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(victims...),
	}), writerNow).Note
}

func TestWritePreemptionNamesThePreemptorPriorityAndTime(t *testing.T) {
	at := time.Date(2026, 3, 1, 10, 2, 0, 0, time.UTC)
	got := preemptionNote(preemptedPod("worker-5f", at))

	assert.Contains(t, got, "worker-5f was preempted by batch/importer-x "+
		"(priority 1000 > 0) at 10:02.")
}

func TestWritePreemptionCountsManyVictims(t *testing.T) {
	at := time.Date(2026, 3, 1, 10, 2, 0, 0, time.UTC)
	got := preemptionNote(preemptedPod("worker-5f", at),
		preemptedPod("worker-6g", at.Add(time.Minute)),
		preemptedPod("worker-7h", at))

	assert.Contains(t, got, "3 pods were preempted by batch/importer-x "+
		"(priority 1000 > 0) at 10:03.")
}

func TestWritePreemptionLinksTheReplacementsThatCannotRun(t *testing.T) {
	at := time.Date(2026, 3, 1, 10, 2, 0, 0, time.UTC)
	stuck := detection.Finding{
		Entity: inventory.CoreID(kube.KindPod, "shop", "worker-8d"),
		Reason: reasons.Unschedulable, Severity: detection.Warning,
		Since:   at,
		Summary: "Pod cannot be scheduled",
		Evidence: []detection.Evidence{{Label: "scheduler",
			Value: "0/2 nodes are available: 2 Insufficient cpu."}},
	}
	got := preemptionNote(preemptedPod("worker-5f", at), stuck)

	assert.Contains(t, got, "worker-5f was preempted by batch/importer-x "+
		"(priority 1000 > 0) at 10:02, and 1 replacement cannot be "+
		"scheduled: \"0/2 nodes are available: 2 Insufficient cpu\".")
}
