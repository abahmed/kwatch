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

func archCrash(evidence ...detection.Evidence) incident.Incident {
	id := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	return incident.Incident{
		ID: "arch-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: writerNow.Add(-time.Hour),
			Summary: "Container crashes on arm64 node n1 with " +
				"\"exec format error\": the image has no arm64 build",
			Evidence: evidence}),
	}
}

func TestWriteArchitectureNamesTheHealthyReplicas(t *testing.T) {
	got := Writer{}.Write(announce(archCrash(
		detection.Evidence{Label: detection.EvidenceArchHealthy,
			Value: "2 pods on amd64 nodes"})), writerNow).Note

	assert.Contains(t, got, "The 2 pods on amd64 nodes run fine.")
}

func TestWriteArchitectureSaysNothingWithoutHealthyReplicas(t *testing.T) {
	got := Writer{}.Write(announce(archCrash()), writerNow).Note

	assert.NotContains(t, got, "run fine")
}
