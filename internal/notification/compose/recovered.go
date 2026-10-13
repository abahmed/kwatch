package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// alsoRecovered introduces the other recoveries a resolve carries.
const alsoRecovered = "Recovered at the same moment:"

// Recovered adds to the resolve msg the recoveries of others, which
// happened in the same tick and have no thread of their own: each lead
// sentence is a line of the note, so one message tells people everything
// that ended. Without others the message is returned as it is.
func Recovered(
	msg notification.Message, others []notification.Message,
) notification.Message {
	if len(others) == 0 {
		return msg
	}
	leads := make([]string, 0, len(others))
	for _, o := range others {
		leads = append(leads, o.Title)
	}
	msg.Lines = append(msg.Lines, alsoRecovered+" "+strings.Join(leads, " "))
	msg.Note += " " + alsoRecovered + " " + strings.Join(leads, " ")
	msg.Doc = append(msg.Doc, notification.Block{Kind: notification.Para,
		Spans: []notification.Span{{Text: alsoRecovered}}})
	for _, lead := range leads {
		msg.Doc = append(msg.Doc, notification.Block{
			Kind: notification.Bullet, Spans: []notification.Span{{Text: lead}}})
	}
	return msg
}
