package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// An acknowledged incident says so once, stays quiet for the eight days
// it is acknowledged (a reminder would be due after seven), and says once
// that the acknowledgement is gone.
func TestAckAnnotationIsToldOnceEachWayAndSilencesReminders(t *testing.T) {
	result := replayNamed(t, "ack-annotation")

	var reasons []incident.Reason
	for _, d := range result.Decisions {
		reasons = append(reasons, d.Reason)
	}
	assert.Equal(t, []incident.Reason{incident.ReasonSettled,
		incident.ReasonAcknowledged, incident.ReasonAckRemoved}, reasons)
	assert.Empty(t, decisionsOf(result, incident.Update,
		incident.ReasonReminder))
	ack := result.Decisions[1].Incident.Ack
	require.NotNil(t, ack)
	assert.Equal(t, "looking into it", ack.Note)
	assert.Equal(t, "deployment", string(ack.On.Kind))
}

// An annotation already on the object at the announcement is mentioned
// in it, and the thread gets no separate acknowledgement.
func TestAckAlreadyPresentIsMentionedInTheAnnouncement(t *testing.T) {
	result := replayNamed(t, "ack-at-announce")

	require.Len(t, result.Messages, 1)
	assert.Contains(t, result.Messages[0].Note,
		"The ack annotation is still present on deployment/payments")
	assert.Empty(t, decisionsOf(result, incident.Update,
		incident.ReasonAcknowledged))
}

// The owner of the namespace routes an incident whose workload has none.
func TestOwnerOfTheNamespaceRoutesTheIncident(t *testing.T) {
	result := replayNamed(t, "owner-routed")

	require.NotEmpty(t, result.Messages)
	for i, m := range result.Messages {
		assert.Equal(t, []string{"payments"}, m.Route.Owners, "message %d", i)
	}
}

// With no owner anywhere the incident has none, and goes to the routes
// that ask for no owner.
func TestIncidentWithoutAnOwnerRoutesWithNone(t *testing.T) {
	result := replayNamed(t, "bad-rollout")

	require.NotEmpty(t, result.Messages)
	for _, m := range result.Messages {
		assert.Empty(t, m.Route.Owners)
	}
}
