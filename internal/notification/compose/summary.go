package compose

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/notification"

	"github.com/abahmed/kwatch/internal/incident"
)

// maxSummaryNamed bounds how many existing problems the startup
// summary describes one by one.
const maxSummaryNamed = 5

// StartupKey is the conversation key of the startup summary sent at at.
// Each session gets its own key, so a restart never reuses the previous
// session's conversation or paging alert.
func StartupKey(at time.Time) string {
	return notification.SummaryKeyPrefix + at.UTC().Format("20060102T150405.000Z")
}

// StartupSummary is one message listing the incidents that already existed
// when kwatch started, instead of one alert each. Its key is derived from
// now; see StartupKey.
//
// Each listed incident keeps its own conversation: its later updates and
// its resolve are sent under the incident's key, not under the summary.
// The summary itself is closed by StartupResolved once every listed
// incident has resolved.
func (w Writer) StartupSummary(
	decisions []incident.Decision, now time.Time,
) notification.Message {
	// The caller's order is its own; the summary sorts a copy.
	decisions = append([]incident.Decision(nil), decisions...)
	sort.SliceStable(decisions, func(i, j int) bool {
		return decisions[i].Incident.Tier > decisions[j].Incident.Tier
	})
	mark, status := notification.MarkerLow, notification.StatusLow
	if len(decisions) > 0 {
		mark = marker(decisions[0].Incident)
		status = tierStatus(decisions[0].Incident.Tier)
	}
	sentences := []sentence{{part: partLead, text: "kwatch" +
		w.clusterTag() + " started and found " +
		plural(len(decisions), "problem") + " that " +
		"began before it was watching."}}
	for i, d := range decisions {
		if i == maxSummaryNamed {
			sentences = append(sentences, sentence{part: partProof,
				text: sentenceCase(numberWord(len(decisions)-i) + " more" +
					" " + verb(len(decisions)-i, "is", "are") +
					" not described here")})
			break
		}
		// The lead named the cluster; the listed titles do not repeat it.
		sentences = append(sentences, sentence{part: partProof,
			text: Writer{}.Write(d, now).Title})
	}
	sentences = append(sentences, sentence{part: partAction,
		text: "Each gets its own message when it changes or resolves."})
	msg := notification.Message{
		Key: StartupKey(now), Revision: 1, Status: status, Opens: true,
	}
	fill(&msg, mark, sentences)
	return msg
}

// StartupResolved closes the startup summary with key once every incident
// it listed has resolved.
func (w Writer) StartupResolved(key string, count int) notification.Message {
	msg := notification.Message{
		Key: key, Revision: 2, Status: notification.StatusResolved,
	}
	text := "all " + plural(count, "problem") + " found when kwatch" +
		w.clusterTag() + " started have resolved"
	if count == 1 {
		text = "the problem found when kwatch" + w.clusterTag() +
			" started has resolved"
	}
	fill(&msg, notification.MarkerResolved, []sentence{{part: partLead,
		text: sentenceCase(text)}})
	return msg
}
