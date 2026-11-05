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

// archIncident is api in shop crashing with "exec format error" on an
// arm64 node; healthy says replicas on other nodes run fine.
func archIncident(healthy string) incident.Incident {
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	container := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "shop", Name: "api-1-x/app"}
	ev := []detection.Evidence{
		{Label: detection.EvidenceError,
			Value: "exec /app/api: exec format error"},
		{Label: detection.EvidenceArchNode, Value: "arm64 node n1"},
		{Label: detection.EvidenceArchImage,
			Value: "registry.example/team/api:1.4"}}
	if healthy != "" {
		ev = append(ev, detection.Evidence{
			Label: detection.EvidenceArchHealthy, Value: healthy})
	}
	return incident.Incident{
		ID: "arch-1", Root: deploy, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-10 * time.Minute),
		Members: members(detection.Finding{Entity: container,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since:    writerNow.Add(-10 * time.Minute),
			Summary:  "Container app crashes on arm64 node n1",
			Evidence: ev}),
	}
}

// The CPU is named outright, release or not.
func TestArchLeadNamesTheCPU(t *testing.T) {
	msg := Writer{}.Write(announce(archIncident("")), writerNow)
	assert.Contains(t, msg.Title, "api in shop can't run: its image "+
		"api:1.4 is not built for this node's CPU (arm64)")

	msg = Writer{}.Write(announce(archIncident("2 pods on amd64 nodes")),
		writerNow)
	assert.Contains(t, msg.Title, "api in shop can't run on arm64 node "+
		"n1: its image api:1.4 is not built for that CPU")
}
