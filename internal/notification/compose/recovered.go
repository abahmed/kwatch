package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// alsoRecovered introduces the other recoveries a resolve carries.
const alsoRecovered = "Recovered at the same moment:"

// alsoDeleted introduces the other deletions a resolve of a deleted
// object carries. A deleted object did not recover, so it is never named
// under alsoRecovered.
const alsoDeleted = "Deleted at the same moment:"

// Recovered adds to the resolve msg the recoveries of others, which
// happened in the same tick and have no thread of their own: each lead
// sentence is a line of the note, so one message tells people everything
// that ended. Without others the message is returned as it is.
func Recovered(
	msg notification.Message, others []notification.Message,
) notification.Message {
	return carried(msg, others, alsoRecovered)
}

// Deleted adds to the resolve msg of a deleted object the resolves of
// others that ended the same way in the same tick and have no thread of
// their own, under "Deleted at the same moment". Without others the
// message is returned as it is.
func Deleted(
	msg notification.Message, others []notification.Message,
) notification.Message {
	return carried(msg, others, alsoDeleted)
}

// carried adds the lead sentences of others to msg under heading.
func carried(
	msg notification.Message, others []notification.Message,
	heading string,
) notification.Message {
	if len(others) == 0 {
		return msg
	}
	leads := make([]string, 0, len(others))
	for _, o := range others {
		leads = append(leads, carriedLead(o))
	}
	msg.Lines = append(msg.Lines, heading+" "+strings.Join(leads, " "))
	msg.Note += " " + heading + " " + strings.Join(leads, " ")
	msg.Doc = append(msg.Doc, notification.Block{Kind: notification.Para,
		Spans: []notification.Span{{Text: heading}}})
	for _, lead := range leads {
		msg.Doc = append(msg.Doc, notification.Block{
			Kind: notification.Bullet, Spans: []notification.Span{{Text: lead}}})
	}
	return msg
}

// carriedLead is the lead of a carried recovery with, for one that
// speaks of the subject its listing named, the root it ended on, and
// what changed just before it recovered: "Node n1 is ready again. Last
// affected: deployment shop/cart."
func carriedLead(o notification.Message) string {
	lead := o.Title
	for _, line := range o.Lines {
		if strings.HasPrefix(line, lastAffectedLead) ||
			strings.HasPrefix(line, beforeRecoveryLead) {
			lead += " " + line
		}
	}
	return lead
}
