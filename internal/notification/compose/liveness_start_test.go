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
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// liveStartIncident is a Deployment api whose liveness kills carry the
// given evidence, blamed on the liveness rule.
func liveStartIncident(
	rule string, ev ...detection.Evidence,
) incident.Incident {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	ct := inventory.CoreID(kube.KindContainer, "shop", "api-4d-a/app")
	return incident.Incident{
		ID: "inc-ls", Root: api, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: ct,
			Reason: reasons.LivenessKilled, Severity: detection.Critical,
			Since:    writerNow.Add(-time.Hour),
			Summary:  "Container keeps being killed by its liveness probe",
			Evidence: ev}),
		Cause: &rootcause.CauseRecord{Root: api, Rule: rule, Score: 0.8,
			Chain: []inventory.EntityID{api, ct}},
	}
}

var startGives = detection.Evidence{
	Label: detection.EvidenceLivenessGives,
	Value: "40s (10s delay + 3 × 10s)"}

func TestLivenessStartSaysWhatTheStartNeeds(t *testing.T) {
	p := liveStartIncident("liveness-shorter-than-start", startGives,
		detection.Evidence{Label: detection.EvidenceUsualStart,
			Value: "75s"},
		detection.Evidence{Label: detection.EvidenceStartSamples,
			Value: "5"})

	text := notification.Text(Writer{}.Write(announce(p), writerNow))

	assert.Contains(t, text, "Liveness gives api 40s (10s delay + 3 × "+
		"10s) but api normally needs about 75s to become ready (last 5 "+
		"starts), so it is killed before it finishes starting. A "+
		"startupProbe or a longer delay would let it start.")
}

func TestLivenessStartWithoutHistoryStatesOnlyTheFact(t *testing.T) {
	p := liveStartIncident("liveness-kills-before-ready", startGives,
		detection.Evidence{Label: detection.EvidenceKilledBeforeReady,
			Value: "true"})

	text := notification.Text(Writer{}.Write(announce(p), writerNow))

	assert.Contains(t, text, "Liveness gives api 40s (10s delay + 3 × "+
		"10s), and api was not ready when each kill came.")
	assert.NotContains(t, text, "normally needs")
	assert.NotContains(t, text, "startupProbe")
}

func TestLivenessStartSaysNothingForAnotherCause(t *testing.T) {
	p := liveStartIncident("self", startGives,
		detection.Evidence{Label: detection.EvidenceUsualStart,
			Value: "75s"})

	text := notification.Text(Writer{}.Write(announce(p), writerNow))

	assert.NotContains(t, text, "Liveness gives")
}
