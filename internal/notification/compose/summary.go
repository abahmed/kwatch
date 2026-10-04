package compose

import (
	"sort"
	"strings"
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

// DigestKey is the conversation key of the digest sent at at. Each digest
// is its own conversation; nothing ever updates or resolves it.
func DigestKey(at time.Time) string {
	return notification.DigestKeyPrefix + at.UTC().Format("20060102T150405.000Z")
}

// Digest is one message for the low-priority incidents of the last
// window: the ones that opened, at their current state, and the ones an
// earlier digest listed that have resolved. None of them interrupts on
// its own.
func (w Writer) Digest(
	opened, resolved []incident.Decision, now time.Time,
) notification.Message {
	opened = append([]incident.Decision(nil), opened...)
	sort.SliceStable(opened, func(i, j int) bool {
		return opened[i].Incident.Tier > opened[j].Incident.Tier
	})
	var counts []string
	if len(opened) > 0 {
		counts = append(counts, plural(len(opened), "low-priority problem"))
	}
	if len(resolved) > 0 {
		counts = append(counts, plural(len(resolved), "earlier one")+
			" that resolved")
	}
	sentences := []sentence{{part: partLead, text: "kwatch" +
		w.clusterTag() + " has " + joinWords(counts) + " to report."}}
	sentences = append(sentences, digestTitles(opened, now, "")...)
	sentences = append(sentences,
		digestTitles(resolved, now, "Resolved: ")...)
	sentences = append(sentences, sentence{part: partAction,
		text: "None of them is urgent; one that gets worse gets its own " +
			"message."})
	msg := notification.Message{
		Key: DigestKey(now), Revision: 1, Status: notification.StatusLow,
		Opens: true,
	}
	fill(&msg, notification.MarkerLow, sentences)
	return msg
}

// digestTitles lists up to maxSummaryNamed incidents by their title.
func digestTitles(
	decisions []incident.Decision, now time.Time, prefix string,
) []sentence {
	var out []sentence
	for i, d := range decisions {
		if i == maxSummaryNamed {
			out = append(out, sentence{part: partProof,
				text: sentenceCase(numberWord(len(decisions)-i) + " more" +
					" " + verb(len(decisions)-i, "is", "are") +
					" not described here")})
			break
		}
		out = append(out, sentence{part: partProof,
			text: prefix + Writer{}.Write(d, now).Title})
	}
	return out
}

// joinWords joins two or three phrases with commas and "and".
func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return "nothing"
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " +
		words[len(words)-1]
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
