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

func neverHealthy(evidence ...detection.Evidence) incident.Incident {
	id := inventory.CoreID(kube.KindDeployment, "shop", "api")
	return incident.Incident{
		ID: "first-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.WorkloadNeverReady, Severity: detection.Warning,
			Since: writerNow.Add(-time.Hour),
			Summary: "Has pods that run without restarting but are not " +
				"ready; 0 of 3 replicas are ready",
			Evidence: evidence}),
	}
}

func TestWriteSaysAFirstRolloutNeverBecameHealthy(t *testing.T) {
	created := writerNow.Add(-12 * time.Minute)
	p := neverHealthy(detection.Evidence{
		Label: detection.EvidenceNeverHealthy,
		Value: created.Format(time.RFC3339)})

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "api in shop has never become healthy "+
		"since it was created 12 minutes ago.")
}

func TestWriteKeepsTheUsualLeadForAWorkloadThatWorkedBefore(t *testing.T) {
	msg := Writer{}.Write(announce(neverHealthy()), writerNow)

	assert.NotContains(t, msg.Note, "never become healthy")
}
