package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// reminderSentences restate an incident that has stayed open: what
// fails, in the words of its own lead and with its cause when one is
// known, then how long, counted from when the problem started:
// "payments in shop keeps crash looping because secret db does not
// exist; still failing, for two weeks now."
func reminderSentences(f caseFacts) []sentence {
	age := humanDuration(f.now.Sub(StartedAt(f.p)))
	lead := leadSentences(f)
	text := strings.TrimSuffix(withoutSpan(lead[0].text), ".")
	lead[0].text = text + "; " + stillWords(f) + ", for " + age + " now."
	return append(lead, strongestProof(f)...)
}

// failingAgainSentences tell that a page that had resolved is back,
// with the count the incident carries (RepeatCount), so consecutive
// returns do not read alike: "api in shop is failing again: 4th time
// in two hours."
func failingAgainSentences(f caseFacts) []sentence {
	subject := leadSubject(f)
	text := f.leadName(subject) + " is failing again"
	if f.p.RepeatCount > 0 {
		text += ": " + humanizeText(incident.RepeatPhrase(f.p.RepeatCount))
	}
	return append([]sentence{{part: partLead,
		text: capitalName(subject, endSentence(text))}},
		strongestProof(f)...)
}

// stillWords say what is still so: "still failing", "still down", or,
// for a condition that fails nothing (a quota used up with nothing
// refused, a container throttled but ready), "still the case".
func stillWords(f caseFacts) string {
	if standing(f.members) {
		return "still the case"
	}
	return "still " + downWord(f.p)
}

// standing reports members that hold nothing that fails: every failure
// among them only degrades, or all of them are configuration advice.
func standing(members []detection.Finding) bool {
	conditions := false
	for _, m := range failing(members) {
		if m.Health != detection.Degraded && m.Health != detection.Healthy {
			return false
		}
		conditions = true
	}
	return conditions || onlyAdvisory(members)
}

// IsStanding reports an incident that is a condition, not a failure: a
// digest lists it as still open instead of still failing.
func IsStanding(p incident.Incident) bool {
	return standing(sortedMembers(p))
}
