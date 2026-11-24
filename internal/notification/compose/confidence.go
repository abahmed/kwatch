package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
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

// certaintyOf is the level a cause is stated at, as
// rootcause.CertaintyOf words it: a cause with a near rival is only
// possible.
func certaintyOf(cause *rootcause.CauseRecord) certainty {
	switch rootcause.CertaintyOf(cause) {
	case rootcause.LevelHigh:
		return certaintyHigh
	case rootcause.LevelLikely:
		return certaintyLikely
	case rootcause.LevelPossible:
		return certaintyPossible
	}
	return certaintyNone
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

// rivalChoice joins the two possible causes of a tie, which the
// closing line of a message names (cause_unconfirmed.go). Two that say the same of
// different objects name both once, so the words that follow cannot be
// read as part of the first: "it or deployment arc/ctl is not running,
// so it removes no finalizers", not "it is not running, so it removes
// no finalizers or deployment arc/ctl is not running, so ...".
func rivalChoice(first, second, words, rivalWords string) string {
	tail := " " + words
	if words == "" || words != rivalWords ||
		!strings.HasSuffix(first, tail) || !strings.HasSuffix(second, tail) {
		return first + " or " + second
	}
	return strings.TrimSuffix(first, tail) + " or " + second
}

// rivalPhrase words one of two possible causes. When the cause is the
// subject itself, "claim gpu-claim is failing" would only repeat the
// lead, so it is told as what the subject lacks: "it has no device
// allocated".
func rivalPhrase(cause *rootcause.CauseRecord, subject inventory.EntityID,
) string {
	if cause.Root == subject && knownRule(cause) {
		if words := causeWordsFor(cause); words != "" {
			return "it " + words
		}
	}
	return causePhrase(cause, subject)
}

// maxCheckedShown bounds the "Proof:" line.
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
		text: "Proof: " + strings.Join(texts, "; ") + "."}}
}
