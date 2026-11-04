package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// What a dead webhook backend does is said as what was seen, never as
// a forecast of what any request will meet.
func TestWebhookConsequenceStatesWhatWasSeen(t *testing.T) {
	for _, reason := range []string{reasons.WebhookNoEndpoints,
		reasons.WebhookBackendNotFound} {
		warning := consequenceSentences(caseFacts{ok: true,
			lead: detection.Finding{Reason: reason,
				Severity: detection.Warning}})
		critical := consequenceSentences(caseFacts{ok: true,
			lead: detection.Finding{Reason: reason,
				Severity: detection.Critical}})

		assert.Equal(t, "It fails closed (failurePolicy Fail); no "+
			"request has been rejected so far.", warning[0].text, reason)
		assert.Equal(t, "It fails closed (failurePolicy Fail) and the API "+
			"server has rejected requests because of it.",
			critical[0].text, reason)
	}
}

// A fail-closed webhook whose rules cover objects that exist now has
// their creates and updates refused by the API server today: that is
// the state of the admission chain, said without waiting for a refusal.
// When the webhook is the root, its own summary already says it.
func TestWebhookConsequenceNamesTheResourcesItRejects(t *testing.T) {
	hook := inventory.CoreID(kube.KindValidatingHook, "", "guard")
	lead := detection.Finding{Entity: hook,
		Reason:   reasons.WebhookBackendNotFound,
		Severity: detection.Warning, Evidence: []detection.Evidence{{
			Label: detection.EvidenceWebhookIntercepts,
			Value: "pods, services"}}}
	service := inventory.CoreID(kube.KindService, "guard", "guard-svc")

	got := consequenceSentences(caseFacts{ok: true, lead: lead,
		p: incident.Incident{Root: service}})
	own := consequenceSentences(caseFacts{ok: true, lead: lead,
		p: incident.Incident{Root: hook}})

	assert.Equal(t, "It fails closed (failurePolicy Fail) and covers "+
		"pods, services that exist now: their creates and updates are "+
		"rejected right now.", got[0].text)
	assert.Empty(t, own)
}
