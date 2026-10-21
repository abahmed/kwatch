package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ongoingState is a digest-tier incident that has stood for a day, whose
// one finding has the given health.
func ongoingState(health detection.Health) incident.Incident {
	quota := inventory.CoreID(kube.KindQuota, "batch", "pod-cap")
	return incident.Incident{ID: "inc-q", Root: quota, Tier: incident.Digest,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(detection.Finding{Entity: quota,
			Reason: "ResourceQuotaExhausted", Health: health,
			Severity: detection.Info, Since: at(0, 0),
			Summary: "Quota is used up (pods)"})}
}

// A condition that degrades nothing is not "still failing".
func TestReminderOfAStandingConditionDoesNotSayFailing(t *testing.T) {
	now := at(0, 0).Add(24 * time.Hour)

	title := Writer{}.OngoingTitle(ongoingState(detection.Degraded), now)

	assert.Contains(t, title, "still the case, for one day now.")
	assert.NotContains(t, title, "failing")
}

func TestReminderOfAFailureSaysItIsStillFailing(t *testing.T) {
	now := at(0, 0).Add(24 * time.Hour)

	title := Writer{}.OngoingTitle(ongoingState(detection.Failing), now)

	assert.Contains(t, title, "still failing, for one day now.")
}
