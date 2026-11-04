package announce

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// restoredWith is a restored notify incident whose findings are back.
func restoredWith(
	reason string, severity detection.Severity,
) incident.Incident {
	root := inventory.CoreID(kube.KindValidatingHook, "", "policy")
	f := detection.Finding{Entity: root, Reason: reason, Severity: severity}
	return incident.Incident{ID: "inc-1", Root: root, Tier: incident.Notify,
		State:   incident.Open,
		Members: map[detection.Key]detection.Finding{f.Key(): f}}
}

// An idle fail-closed webhook was announced once and then waits for
// the digest: a restart lists it with the lower-priority incidents, not
// at the notify tier again.
func TestRestoredIdleWebhookIsNotNewsAgain(t *testing.T) {
	idle := restoredWith(reasons.WebhookBackendNotFound, detection.Warning)
	assert.True(t, idle.DigestBound())
	assert.False(t, currentNews(idle))

	refused := restoredWith(reasons.WebhookBackendNotFound,
		detection.Critical)
	assert.False(t, refused.DigestBound(), "a refused request is news")
}
