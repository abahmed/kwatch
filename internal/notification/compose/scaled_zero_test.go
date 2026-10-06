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

func scaledToZero(evidence ...detection.Evidence) incident.Incident {
	id := inventory.CoreID(kube.KindDeployment, "shop", "api")
	return incident.Incident{
		ID: "zero-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.ScaledToZeroRouted, Severity: detection.Warning,
			Since: writerNow.Add(-time.Hour),
			Summary: "Is scaled to 0 but Ingress shop/web still routes " +
				"traffic to it via Service api",
			Evidence: evidence}),
	}
}

func TestWriteScaledToZeroNamesTheRouteWhoAndWhen(t *testing.T) {
	scaledAt := time.Date(2024, 1, 15, 9, 10, 0, 0, time.UTC)
	p := scaledToZero(
		detection.Evidence{Label: detection.EvidenceScaledBy,
			Value: "kubectl-scale"},
		detection.Evidence{Label: detection.EvidenceScaledAt,
			Value: scaledAt.Format(time.RFC3339)})

	msg := Writer{}.Write(announce(p), writerNow)

	assert.Contains(t, msg.Note, "api in shop is scaled to 0 "+
		"but Ingress shop/web still routes traffic to it via Service api.")
	assert.Contains(t, msg.Note,
		"It was scaled to 0 by kubectl-scale at 09:10.")
}

func TestWriteScaledToZeroWithoutHistoryAddsNoWho(t *testing.T) {
	msg := Writer{}.Write(announce(scaledToZero()), writerNow)

	assert.NotContains(t, msg.Note, "It was scaled to 0 by")
	assert.NotContains(t, msg.Note, "It was scaled to 0 at")
}
