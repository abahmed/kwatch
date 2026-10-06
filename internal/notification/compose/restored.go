package compose

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// RestoredSummary is one message listing the incidents announced before
// a restart that still fail once kwatch is back. A restart announces no
// incident one by one, so this is how people learn they are still open.
// Each listed incident keeps its own conversation; the summary names them
// and is never resolved.
func (w Writer) RestoredSummary(
	decisions []incident.Decision, now time.Time,
) notification.Message {
	decisions = append([]incident.Decision(nil), decisions...)
	sortByImpact(decisions)
	mark, status := notification.MarkerLow, notification.StatusLow
	if len(decisions) > 0 {
		mark = marker(decisions[0].Incident)
		status = tierStatus(decisions[0].Incident.Tier)
	}
	lead := "kwatch" + w.clusterTag() + " restarted and " +
		plural(len(decisions), "problem") + " from before " +
		verb(len(decisions), "is", "are") + " still failing."
	sentences := []sentence{{part: partLead, text: lead}}
	sentences = append(sentences, digestTitles(decisions, now, "")...)
	sentences = append(sentences, sentence{part: partAction,
		text: eachOwnMessage})
	msg := notification.Message{
		Key: StartupKey(now), Revision: 1, Status: status, Opens: true,
		Route: summaryRoute(decisions, nil),
	}
	fill(&msg, mark, sentences)
	return msg
}
