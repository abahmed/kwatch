package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Proof weights rank the facts that make a cause convincing. Rule
// points carry their own weights, which fall between these.
const (
	weightChange    = 0.8
	weightUsage     = 0.6
	weightScheduler = 0.55
	weightError     = 0.28
)

// maxQuote bounds a quoted error or scheduler message.
const maxQuote = 120

// weightCoOnset ranks the moment several workloads began failing.
const weightCoOnset = 0.6

// changeSentences say what the blamed change did: "The release changed
// the image from payments:2.2 to payments:2.3." When the lead does not
// blame the change, they say when it happened: "Its selector changed
// at 14:02 from app=payments to app=payment." Secret values are never
// quoted.
func changeSentences(f caseFacts) []sentence {
	if f.p.Cause == nil || f.p.Cause.Change == nil ||
		systemChange(*f.p.Cause.Change) || hasRevisionEdits(f) ||
		causeRuleIs(f, heldVolumeRules...) ||
		causeRuleIs(f, explain.RuleNodeAgentAbsent,
			explain.RuleClaimSelectedNodeGone) ||
		hasConfigVersion(f) ||
		policyBlocksCall(f.p.Cause) {
		return nil
	}
	change := *f.p.Cause.Change
	text := ""
	switch {
	case ownCause(f.p.Cause) && scaleOnly(change):
		text = scaledText(change)
	case ownCause(f.p.Cause) && len(change.Fields) > 0:
		text = "the " + clock(change.At) + " release changed " +
			fieldList(change) + changeValues(change)
	case blamedChange(f) != nil && previousSaidByLead(f):
		text = releasedByText(change)
	case blamedChange(f) != nil:
		text = blamedChangeText(change)
	default:
		text = changeTimeText(f, change)
	}
	if text == "" {
		return nil
	}
	// An actor's name keeps its case ("bob"); other sentences start
	// with a capital.
	if change.Actor == "" || !strings.HasPrefix(text, change.Actor) {
		text = upperFirst(text)
	}
	return []sentence{{part: partProof, weight: weightChange,
		text: endSentence(text + changeCauseText(change))}}
}

// changeCauseText quotes the reason recorded with the change, when the
// person or tool that made it left one: `, recorded as "bump to 2.4"`.
func changeCauseText(change inventory.Change) string {
	if change.Cause == "" {
		return ""
	}
	return ", recorded as " + quoted(change.Cause)
}

// blamedChangeText adds to a lead that already named the change: the
// image a release replaced, or the fields a change touched.
func blamedChangeText(change inventory.Change) string {
	if len(change.Fields) == 0 {
		return ""
	}
	field := change.Fields[0]
	if releasedImage(change) == field.After && field.Before != "" &&
		len(change.Fields) == 1 {
		// The lead named the new image already.
		if change.Actor != "" {
			return change.Actor + " released it; the previous image was " +
				field.Before
		}
		return "The previous image was " + field.Before
	}
	if change.Actor != "" {
		return change.Actor + " changed " + fieldList(change) +
			changeValues(change)
	}
	if scaleOnly(change) {
		return "the replicas were scaled" + changeValues(change)
	}
	if incident.IsWorkload(change.Entity.Kind) {
		return "the release changed " + fieldList(change) +
			changeValues(change)
	}
	return fieldList(change) + " changed" + changeValues(change)
}

// changeTimeText says when the root's own change happened, for a lead
// about the root's condition: "its selector changed at 14:02".
func changeTimeText(f caseFacts, change inventory.Change) string {
	at := " at " + clock(change.At)
	who, whose := "it", "its "
	if change.Entity != f.p.Root {
		who = shortName(change.Entity)
		whose = who + "'s "
	}
	switch {
	case change.Created:
		return who + " was created" + at
	case change.Deleted:
		return who + " was deleted" + at
	case len(change.Fields) == 0:
		return who + " changed" + at
	}
	return whose + fieldNoun(change.Fields[0].Path) + " changed" + at +
		changeValues(change)
}

// fieldList names the changed fields: "the image", "key app.yaml",
// "keys db-pass and db-password" or "the image and two other fields".
func fieldList(change inventory.Change) string {
	keys := make([]string, 0, len(change.Fields))
	for _, field := range change.Fields {
		if key, ok := dataKey(field.Path); ok {
			keys = append(keys, key)
		}
	}
	if len(keys) == len(change.Fields) && len(keys) > 1 {
		return "keys " + joinAnd(keys, maxNamed)
	}
	text := fieldWords(change.Fields[0].Path)
	if extra := len(change.Fields) - 1; extra > 0 {
		text += " and " + plural(extra, "other field")
	}
	return text
}

// changeValues is " from a to b" for one readable field, else "".
// Secret values, hashes, generations and hidden values are left out, and
// so is the added, changed or removed state of a config key.
func changeValues(change inventory.Change) string {
	if len(change.Fields) != 1 || change.Entity.Kind == kube.KindSecret {
		return ""
	}
	field := change.Fields[0]
	if hiddenEdit(field) {
		return ""
	}
	if field.Before == "" {
		return setValue(change)
	}
	if opaque(field.Before) || opaque(field.After) {
		return ""
	}
	if limitRangeChange(change) {
		before, after := limitRangeValues(field.Before, field.After)
		return " from " + before + " to " + after
	}
	return " from " + field.Before + " to " + field.After
}

// setValue is " to enforce=restricted" for a field that was not set
// before; "" for any other change, which changeValues words itself.
func setValue(change inventory.Change) string {
	if len(change.Fields) != 1 || change.Entity.Kind == kube.KindSecret {
		return ""
	}
	field := change.Fields[0]
	// A config key with no old value only says added, changed or removed.
	if _, key := dataKey(field.Path); key || field.Before != "" ||
		opaque(field.After) || hiddenEdit(field) {
		return ""
	}
	return " to " + field.After
}

// hiddenEdit reports an edit with a value that may not be shown.
func hiddenEdit(field inventory.FieldChange) bool {
	return field.Before == inventory.HiddenValue ||
		field.After == inventory.HiddenValue
}

// opaqueValue matches values that mean nothing to a reader: a template
// hash or an object generation.
var opaqueValue = regexp.MustCompile(`^([0-9a-f]{8,}|generation \d+)$`)

func opaque(value string) bool {
	return value == "" || opaqueValue.MatchString(value)
}

func actorOr(actor, fallback string) string {
	if actor == "" {
		return fallback
	}
	return actor
}

// causeProofSentences are the cause's supporting proof, such as "Only pods
// of the new revision fail."
func causeProofSentences(f caseFacts) []sentence {
	if f.p.Cause == nil || causeRuleIs(f, heldVolumeRules...) ||
		f.p.Cause.Root.Kind == explain.KindFailureSignature {
		// A shared error has no evidence beyond what its lead says.
		return nil
	}
	var out []sentence
	for _, item := range f.p.Cause.Proof {
		text := proofWords(item)
		if skipProof(f, item, text) {
			continue
		}
		next := sentence{part: partProof, weight: item.Weight,
			text: sentenceCase(humanizeText(text))}
		if item.Code == rootcause.ProofCoOnset {
			// When the workloads began failing is the point of a
			// cause found by timing: it ranks above the other proof.
			next.weight = weightCoOnset
		}
		if item.Code == rootcause.ProofErrorsName {
			var ok bool
			if next, ok = mentionWithError(f, item, next.text); !ok {
				continue
			}
		}
		out = append(out, next)
	}
	return out
}

// skipProof reports a proof item the note leaves out: evidence against
// the cause, an empty wording, or a fact another sentence already says.
func skipProof(f caseFacts, item rootcause.Proof, text string) bool {
	cause := f.p.Cause
	switch {
	case !item.Supports || text == "":
		return true
	case item.Code == rootcause.ProofFailedAfterChange &&
		createdBlame(cause):
		// Failing right after being created says nothing about the
		// creation.
		return true
	case deniedCause(cause) && strings.Contains(text, "mention"):
		return true
	case restatedByStartLead(f, item.Code):
		return true
	}
	// The change sentence already says what changed.
	return (cause.Change != nil &&
		strings.HasSuffix(text, "changed shortly before")) ||
		(policyBlocksCall(cause) &&
			strings.HasSuffix(text, "shortly before"))
}
