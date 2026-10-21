package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
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

// changeSentences say what the blamed change did: "The release changed
// the image from payments:2.2 to payments:2.3." When the lead does not
// blame the change, they say when it happened: "Its selector changed
// at 14:02 from app=payments to app=payment." Secret values are never
// quoted.
func changeSentences(f caseFacts) []sentence {
	if f.p.Cause == nil || f.p.Cause.Change == nil ||
		systemChange(*f.p.Cause.Change) || hasRevisionEdits(f) ||
		causeRuleIs(f, heldVolumeRules...) ||
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
	}
	// The change sentence already says what changed.
	return (cause.Change != nil &&
		strings.HasSuffix(text, "changed shortly before")) ||
		(policyBlocksCall(cause) &&
			strings.HasSuffix(text, "shortly before"))
}

// createdBlame reports a cause whose blamed change is the creation of
// the failing workload itself. A new policy or config that broke
// others still failed them after it changed.
func createdBlame(cause *rootcause.CauseRecord) bool {
	return cause.Change != nil && cause.Change.Created &&
		incident.IsWorkload(cause.Change.Entity.Kind)
}

// errorSentences quote the failure's own error, else the first error
// line investigation found, else the last line the application wrote.
func errorSentences(f caseFacts) []sentence {
	if !f.ok || hasProof(nodeWhySentences(f)) {
		return nil
	}
	said := evidence(f.lead, "error", "message")
	if kube.IsRuntimeLogFailure(said) {
		// The kubelet failing to read the log is not the app's error.
		said = ""
	}
	switch {
	case pullNoise.MatchString(said):
		if !strings.Contains(leadText(f), " pull") {
			return []sentence{{part: partProof, weight: weightError,
				text: "Its image cannot be pulled."}}
		}
	case said != "" && quotedByMissingService(f, said):
		return nil
	case said != "" && !restartNoise.MatchString(said):
		return []sentence{{part: partProof, weight: weightError,
			text: errorIntro(f) + " " + quoted(said) + "."}}
	}
	if first := investigatedError(f); first != nil {
		return first
	}
	if last := lastLine(f.output); last != "" {
		return []sentence{{part: partProof, weight: weightError,
			text: "Its last output was " + quoted(last) + "."}}
	}
	return nil
}

// usageSentences state how full something is: "It is at 94% of 20 GiB."
// or "etl uses about 11 GiB."
func usageSentences(f caseFacts) []sentence {
	var out []sentence
	for _, s := range f.members {
		used := evidence(s, "used")
		if used == "" {
			continue
		}
		subject := "It"
		if s.Entity != f.lead.Entity {
			subject = nameFrom(f.p.Root, s.Entity)
		}
		text := subject + " uses " + humanBytes(used)
		if strings.HasSuffix(used, "%") {
			text = subject + " is at " + usedOfWords(used, s)
		}
		out = append(out, sentence{part: partProof, weight: weightUsage,
			text: endSentence(text + volumeUsersTail(s))})
	}
	return out
}

// schedulerSentences say why the scheduler rejects every node: the
// investigated node counts, else the scheduler's own words.
func schedulerSentences(f caseFacts) []sentence {
	if counted := investigatedScheduler(f); counted != nil {
		return counted
	}
	out := schedulerNumbers(f)
	for _, s := range f.members {
		if said := evidence(s, "scheduler"); said != "" {
			return append(out, sentence{part: partProof,
				weight: weightScheduler,
				text:   "The scheduler says " + quoted(said) + "."})
		}
	}
	return out
}

// reasonConsequences say what the lead's own condition breaks.
var reasonConsequences = map[string]string{
	reasons.TLSCertExpired:         "Clients are rejecting it now.",
	reasons.WebhookNoEndpoints:     webhookRejected,
	reasons.WebhookBackendNotFound: webhookRejected,
}

// webhookRejected says what a webhook with a dead backend did: it
// fails closed, and the API server rejected requests because of it.
const webhookRejected = "It fails closed (failurePolicy Fail) and the " +
	"API server has rejected requests because of it."

// consequenceSentences say what the lead's condition breaks. A webhook
// blocks requests only when its failurePolicy is Fail: critical once a
// request was refused, a warning before, and then it says so.
// An expired certificate only has clients rejecting it when something
// uses it, which is also what makes it critical; one nothing references
// is stated as a fact and breaks nothing.
func consequenceSentences(f caseFacts) []sentence {
	text, ok := reasonConsequences[f.lead.Reason]
	if !f.ok || !ok {
		return nil
	}
	webhook := f.lead.Reason == reasons.WebhookNoEndpoints ||
		f.lead.Reason == reasons.WebhookBackendNotFound
	expired := f.lead.Reason == reasons.TLSCertExpired
	if webhook && f.lead.Severity == detection.Warning {
		// A fail-closed webhook nothing was refused by yet.
		return []sentence{{part: partConsequence, weight: 1,
			text: "It fails closed (failurePolicy Fail); no request " +
				"has been rejected so far."}}
	}
	if (webhook || expired) && f.lead.Severity != detection.Critical {
		return nil
	}
	return []sentence{{part: partConsequence, weight: 1, text: text}}
}
