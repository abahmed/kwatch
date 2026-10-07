package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
)

// An acknowledgement update says something new although the incident's
// fingerprint did not change, so it is not an unchanged update.
func TestAckUpdatesHaveContentOfTheirOwn(t *testing.T) {
	p := incident.Incident{Digest: "abc"}
	acked := contentHash(incident.Decision{Incident: p,
		Reason: incident.ReasonAcknowledged})
	removed := contentHash(incident.Decision{Incident: p,
		Reason: incident.ReasonAckRemoved})
	material := contentHash(incident.Decision{Incident: p,
		Reason: incident.ReasonMaterialChange})

	assert.NotEqual(t, material, acked)
	assert.NotEqual(t, material, removed)
	assert.NotEqual(t, acked, removed)
}
