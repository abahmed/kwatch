package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestWriteFailingAgainCarriesTheReturnCount(t *testing.T) {
	p := podCrash(incident.Notify)
	p.RepeatCount = 4
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonFailingAgain}

	msg := Writer{}.Write(d, writerNow.Add(time.Minute))

	assert.Contains(t, msg.Note,
		"is failing again: 4th time in two hours.")
}

func TestWriteFailingAgainWithoutANoteStillSaysSo(t *testing.T) {
	d := incident.Decision{Action: incident.Update,
		Incident: podCrash(incident.Notify),
		Reason:   incident.ReasonFailingAgain}

	msg := Writer{}.Write(d, writerNow)

	assert.Contains(t, msg.Note, "is failing again.")
}

// aboutEntity is the Entity of a timeline event about one member.
func aboutEntity(id inventory.EntityID) *inventory.EntityID { return &id }
