package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A cause kwatch could not confirm (rootcause.Confirmed: possible,
// with a near rival or nothing to corroborate it) is not named as the
// cause. The message says the symptom and that no cause was confirmed,
// says what was checked, and only its closing small print names the
// candidate, as a possibility. Saying "unknown" costs the reader less
// than a wrong cause stated with confidence.

// unconfirm takes a cause kwatch could not confirm out of the facts and
// keeps it as a possibility. The lead becomes a failure of the symptom,
// not the candidate's own state, and the subject is the candidate only
// when it fails in its own right or its own pods do.
func (f *caseFacts) unconfirm() {
	cause := f.p.Cause
	if !unconfirmedCause(cause) {
		return
	}
	f.possible = cause
	if own := rootFinding(f.p, f.members); own != nil &&
		cause.Root == f.p.Root {
		// The candidate fails in its own right: that failure is the
		// symptom, only why it fails is not confirmed.
		f.symptom, f.lead, f.ok = f.p.Root, *own, true
		f.p.Cause = nil
		return
	}
	others := failuresBesides(f.members, cause)
	if f.ok && f.lead.Entity == cause.Root && len(others) > 0 {
		f.lead = leadFailure(f.p.Root, causeFirst(f.p, others))
	}
	f.symptom = symptomSubject(*f)
	if f.symptom == cause.Root && f.ok && f.lead.Entity != cause.Root &&
		!f.owns(cause.Root, f.lead.Entity) {
		f.symptom = f.lead.Entity
		if owner, ok := ownerIn(f.p.Impact, f.symptom); ok {
			f.symptom = owner
		}
	}
	f.p.Cause = nil
}

// unconfirmedCause reports a cause a message must not name as the
// cause (rootcause.Confirmed), unless its lead only states facts.
func unconfirmedCause(cause *rootcause.CauseRecord) bool {
	return cause != nil && !rootcause.Confirmed(cause) &&
		!factWorded(cause)
}

// factWorded reports a cause whose lead states only what was seen, so
// it names no cause to confirm: the subject failing on its own, a zone
// or pool failing as a whole, a request a webhook denied, or a failure
// signature the pods share. Their leads never named two causes either.
func factWorded(cause *rootcause.CauseRecord) bool {
	kind := cause.Root.Kind
	return selfCause(cause) || selfCause(cause.Rival) ||
		kind == kube.KindZone || kind == kube.KindNodePool ||
		deniedCause(cause) || kind == explain.KindFailureSignature
}

// failuresBesides are the failing members other than the candidate's
// own findings.
func failuresBesides(
	members []detection.Finding, cause *rootcause.CauseRecord,
) []detection.Finding {
	var out []detection.Finding
	for _, m := range failing(members) {
		if m.Entity != cause.Root {
			out = append(out, m)
		}
	}
	return out
}

// unconfirmedLead says the symptom and that no cause was confirmed:
// "intake in mail is failing; kwatch could not confirm a cause".
func unconfirmedLead(f caseFacts) (string, bool) {
	if f.possible == nil {
		return "", false
	}
	state := stateWords(f.p)
	if f.ok && f.owns(f.symptom, f.lead.Entity) {
		if own := beforeColon(leadState(f)); own != "" &&
			(f.symptom == f.possible.Root ||
				!strings.Contains(own, f.possible.Root.Name)) {
			state = own
		}
	}
	return f.leadName(f.symptom) + " " + state +
		"; kwatch could not confirm a cause", true
}

// unconfirmedSentences close the message: what kwatch checked, and the
// candidate as a possibility (both candidates when two scored alike).
func unconfirmedSentences(f caseFacts) []sentence {
	cause := f.possible
	if cause == nil {
		return nil
	}
	var out []sentence
	if checked := checkedTexts(cause.Checked); len(checked) > 0 {
		out = append(out, sentence{part: partChecked,
			text: "kwatch checked: " + strings.Join(checked, "; ") + "."})
	}
	first := rivalPhrase(cause, f.symptom)
	text := "A possibility kwatch could not confirm: " + first
	if cause.Rival != nil {
		if second := rivalPhrase(cause.Rival, f.symptom); second != first {
			text = "Possibilities kwatch could not confirm: " +
				rivalChoice(first, second, causeWordsFor(cause),
					causeWordsFor(cause.Rival))
		}
	}
	return append(out, sentence{part: partChecked,
		text: upperFirst(endSentence(text))})
}

// checkedTexts are the counterfactual checks of a cause, trimmed, at
// most maxCheckedShown of them.
func checkedTexts(checked []string) []string {
	var out []string
	for _, text := range checked {
		text = strings.TrimRight(strings.TrimSpace(text), ".")
		if text != "" {
			out = append(out, text)
		}
		if len(out) == maxCheckedShown {
			break
		}
	}
	return out
}
