package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
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
