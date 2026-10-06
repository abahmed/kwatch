package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// This file words a cause by how sure it is. The levels are the
// scorecard's: high causes were right about nine times in ten, likely
// ones were right most of the time, and below that kwatch only
// suggests. A message never sounds surer than its level.

// certainty is how sure the cause is; it chooses the lead's wording.
type certainty uint8

const (
	certaintyNone certainty = iota
	certaintyPossible
	certaintyLikely
	certaintyHigh
)

// certaintyOf is the level a cause is stated at. A cause with a rival
// of nearly the same score is only possible: kwatch cannot tell them
// apart, so it must not pick one.
func certaintyOf(cause *rootcause.CauseRecord) certainty {
	switch {
	case cause == nil:
		return certaintyNone
	case cause.Rival != nil:
		return certaintyPossible
	case cause.Score >= rootcause.High:
		return certaintyHigh
	case cause.Score >= rootcause.Likely:
		return certaintyLikely
	default:
		return certaintyPossible
	}
}

// word is the level's name, as the scorecard calls it.
func (c certainty) word() string {
	switch c {
	case certaintyHigh:
		return "high"
	case certaintyLikely:
		return "likely"
	case certaintyPossible:
		return "possible"
	}
	return ""
}

// causeLink joins a symptom to its cause: "api is down because the
// secret is missing", "..., likely because ..." or "...; <cause>,
// which might be related".
func causeLink(cause *rootcause.CauseRecord, symptom, why string) string {
	switch certaintyOf(cause) {
	case certaintyHigh:
		return symptom + " because " + why
	case certaintyLikely:
		return symptom + ", likely because " + why
	}
	return symptom + "; " + why + ", which might be related"
}

// changeLink joins a symptom to the change blamed for it: "after the
// 14:02 release", "..., likely caused by ..." or "...; it might be
// related to ...".
func changeLink(cause *rootcause.CauseRecord, symptom, change string,
) string {
	switch certaintyOf(cause) {
	case certaintyHigh:
		return symptom + " after " + change
	case certaintyLikely:
		return symptom + ", likely caused by " + change
	}
	return symptom + "; it might be related to " + change
}

// rivalLead says two causes scored almost alike: "api in shop is
// failing; two possible causes: node n1 is low on memory or secret
// db-creds does not exist". It is not used where the cause has its own
// wording (a zone, a failure signature, a denied request, itself).
func rivalLead(f caseFacts) (string, bool) {
	cause := f.p.Cause
	if cause == nil || cause.Rival == nil || selfCause(cause) ||
		selfCause(cause.Rival) || leadIsGroup(f) || deniedCause(cause) ||
		cause.Root.Kind == explain.KindFailureSignature {
		return "", false
	}
	subject := symptomSubject(f)
	first := causePhrase(cause, subject)
	second := causePhrase(cause.Rival, subject)
	if first == second {
		return "", false
	}
	return symptomState(f, subject) + "; two possible causes: " + first +
		" or " + second, true
}

// maxCheckedShown bounds the "Checked:" line.
const maxCheckedShown = 3

// checkedLineSentences are the closing small print of a message with a
// cause: what the counterfactual checks found, such as "image api:2.3
// runs fine in staging-2". Without such checks there is no line.
func checkedLineSentences(f caseFacts) []sentence {
	if f.p.Cause == nil || len(f.p.Cause.Checked) == 0 {
		return nil
	}
	var texts []string
	for _, text := range f.p.Cause.Checked {
		text = strings.TrimRight(strings.TrimSpace(text), ".")
		if text != "" {
			texts = append(texts, text)
		}
		if len(texts) == maxCheckedShown {
			break
		}
	}
	if len(texts) == 0 {
		return nil
	}
	return []sentence{{part: partChecked,
		text: "Checked: " + strings.Join(texts, "; ") + "."}}
}
