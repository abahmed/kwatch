package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func ackedPod(note string, atAnnounce bool) incident.Incident {
	p := podCrash(incident.Notify)
	p.Ack = &incident.Ack{
		On:   inventory.CoreID("deployment", "default", "api"),
		Note: note, AtAnnounce: atAnnounce,
	}
	return p
}

func TestAcknowledgedUpdateQuotesTheNote(t *testing.T) {
	d := incident.Decision{Action: incident.Update,
		Incident: ackedPod("looking into it", false),
		Reason:   incident.ReasonAcknowledged}

	msg := Writer{}.Write(d, writerNow)

	assert.Contains(t, msg.Note,
		`Acknowledged on deployment/api: "looking into it".`)
	assert.NotContains(t, msg.Note, "crash looping",
		"the update says only the acknowledgement")
}

func TestAcknowledgedUpdateWithoutANote(t *testing.T) {
	d := incident.Decision{Action: incident.Update,
		Incident: ackedPod("", false), Reason: incident.ReasonAcknowledged}

	msg := Writer{}.Write(d, writerNow)

	assert.Contains(t, msg.Note, "Acknowledged on deployment/api.")
}

func TestAcknowledgedNoteLosesCredentials(t *testing.T) {
	d := incident.Decision{Action: incident.Update,
		Incident: ackedPod("password=hunter2 rotating", false),
		Reason:   incident.ReasonAcknowledged}

	msg := Writer{}.Write(d, writerNow)

	assert.NotContains(t, msg.Note, "hunter2")
}

func TestAckRemovedUpdate(t *testing.T) {
	d := incident.Decision{Action: incident.Update,
		Incident: podCrash(incident.Notify),
		Reason:   incident.ReasonAckRemoved}

	msg := Writer{}.Write(d, writerNow)

	assert.Contains(t, msg.Note, "Acknowledgement removed for ")
}

func TestAnnouncementMentionsAnAckThatWasAlreadyThere(t *testing.T) {
	d := incident.Decision{Action: incident.Announce,
		Incident: ackedPod("old note", true), Reason: incident.ReasonSettled}

	msg := Writer{}.Write(d, writerNow)

	assert.Contains(t, msg.Note,
		"The ack annotation is still present on deployment/api")
}

func TestAnnouncementSaysNothingOfALaterAck(t *testing.T) {
	d := incident.Decision{Action: incident.Announce,
		Incident: ackedPod("later", false), Reason: incident.ReasonSettled}

	msg := Writer{}.Write(d, writerNow)

	assert.NotContains(t, msg.Note, "ack annotation")
}

func TestAckUpdatesNameTheCluster(t *testing.T) {
	acked := incident.Decision{Action: incident.Update,
		Incident: ackedPod("on it", false),
		Reason:   incident.ReasonAcknowledged}
	removed := incident.Decision{Action: incident.Update,
		Incident: podCrash(incident.Notify),
		Reason:   incident.ReasonAckRemoved}
	w := Writer{Cluster: "prod-eu-1"}

	assert.Contains(t, w.Write(acked, writerNow).Note,
		`Acknowledged on deployment/api (prod-eu-1): "on it".`)
	assert.Contains(t, w.Write(removed, writerNow).Note, "(prod-eu-1)")
}
