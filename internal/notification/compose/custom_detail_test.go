package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// customFailing is a custom resource reporting Ready=False with message
// and, when event is set, a newest Warning event.
func customFailing(
	kind inventory.Kind, namespace, reason, message, event string,
) incident.Incident {
	id := inventory.CoreID(kind, namespace, "main")
	ev := []detection.Evidence{{Label: "message", Value: message}}
	if event != "" {
		ev = append(ev, detection.Evidence{Label: "event", Value: event})
	}
	return incident.Incident{
		ID: "cr-1", Root: id, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(detection.Finding{Entity: id,
			Reason: reasons.CustomResourceFailure, Severity: detection.Warning,
			Since:   writerNow.Add(-time.Hour),
			Summary: "Reports Ready=False (" + reason + ")", Evidence: ev}),
	}
}

// A nested error keeps its innermost, most specific clause.
func TestCustomResourceStateCarriesTheSpecificError(t *testing.T) {
	p := customFailing("issuer", "", "ErrRegisterAccount",
		"Failed to register account: 400 urn:err:invalidContact: "+
			"Error validating contact(s) :: contact email has forbidden "+
			"domain \"example.org\"", "")
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "is not ready: ")
	assert.Contains(t, msg.Title, "contact email has forbidden domain")
	assert.NotContains(t, msg.Title, "err register account")
}

// A message that only says something failed leaves the why to the
// newest Warning event.
func TestCustomResourceStateFallsBackToTheEvent(t *testing.T) {
	p := customFailing("syncedsecret", "shop", "SyncError",
		"could not get secret data from provider",
		"Secret does not exist")
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "is not ready: ")
	assert.Contains(t, msg.Title, "Secret does not exist")
	assert.NotContains(t, msg.Title, "sync error")
}
