package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// recoveredWords say a checked cause works again, by the kind of its
// root. Only the kinds incident.CheckCause looks at have words.
var recoveredWords = map[inventory.Kind]string{
	kube.KindNode:                "is ready again",
	explain.KindExternalEndpoint: "accepts connections again",
	kube.KindService:             "has ready endpoints again",
}

// checkedCause is the incident's stated cause when the resolve looked at
// it again and the cause is not the subject itself.
func checkedCause(f caseFacts, subject inventory.EntityID) bool {
	cause := f.p.Cause
	return cause != nil && cause.Rule != "self" && cause.Root != subject &&
		f.causeState != incident.CauseUnchecked && !removed(f, subject)
}

// stillClause follows "is healthy again" when the cause is still there:
// ", but db.example.com:5432 still refuses connections". It states what
// the check saw now and nothing about what comes next.
func stillClause(f caseFacts, subject inventory.EntityID) string {
	if f.causeState != incident.CauseStill || !checkedCause(f, subject) {
		return ""
	}
	cause := f.p.Cause
	return ", but " + causeRootName(cause, subject) + " " +
		presentStill(causeWordsFor(cause))
}

// presentStill puts "still" in the present-tense words of a cause: "is
// failing" becomes "is still failing", "refuses connections" becomes
// "still refuses connections".
func presentStill(words string) string {
	for _, be := range []string{"is ", "are "} {
		if rest, ok := strings.CutPrefix(words, be); ok {
			return be + "still " + rest
		}
	}
	return "still " + words
}

// causeStateSentences say what the check found: the cause is still
// present, or it works again.
func causeStateSentences(f caseFacts, subject inventory.EntityID,
) []sentence {
	if !checkedCause(f, subject) {
		return nil
	}
	cause := f.p.Cause
	switch f.causeState {
	case incident.CauseStill:
		return []sentence{{part: partProof,
			text: "The same cause is still present."}}
	case incident.CauseRecovered:
		if words, ok := recoveredWords[cause.Root.Kind]; ok {
			return []sentence{{part: partProof, text: upperFirst(
				causeRootName(cause, subject)+" "+words) + "."}}
		}
	}
	return nil
}

// stillSteps offer one read-only check of a cause that is still there.
func stillSteps(f caseFacts) []notification.Step {
	if f.causeState != incident.CauseStill || f.p.Cause == nil {
		return nil
	}
	steps := nextSteps(f.p, f.members)
	if len(steps) == 0 {
		return nil
	}
	steps[0].Text = "Check it"
	return steps[:1]
}
