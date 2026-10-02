package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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
		systemChange(*f.p.Cause.Change) {
		return nil
	}
	change := *f.p.Cause.Change
	text := ""
	switch {
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
		text: endSentence(text)}}
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
	if isWorkload(change.Entity.Kind) {
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
// Secret values, hashes and generations are left out.
func changeValues(change inventory.Change) string {
	if len(change.Fields) != 1 || change.Entity.Kind == kube.KindSecret {
		return ""
	}
	field := change.Fields[0]
	if opaque(field.Before) || opaque(field.After) {
		return ""
	}
	return " from " + field.Before + " to " + field.After
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
	if f.p.Cause == nil ||
		f.p.Cause.Root.Kind == explain.KindFailureSignature {
		// A shared error has no evidence beyond what its lead says.
		return nil
	}
	var out []sentence
	for _, item := range f.p.Cause.Proof {
		text := proofWords(item)
		if !item.Supports || text == "" || (f.p.Cause.Change != nil &&
			strings.HasSuffix(text, "changed shortly before")) {
			// The change sentence already says what changed.
			continue
		}
		out = append(out, sentence{part: partProof,
			weight: item.Weight,
			text:   sentenceCase(humanizeText(text))})
	}
	return out
}

// errorSentences quote the failure's own error, else the first error
// line investigation found, else the last line the application wrote.
func errorSentences(f caseFacts) []sentence {
	if !f.ok {
		return nil
	}
	said := evidence(f.lead, "error", "message")
	switch {
	case pullNoise.MatchString(said):
		if !strings.Contains(leadText(f), " pull") {
			return []sentence{{part: partProof, weight: weightError,
				text: "Its image cannot be pulled."}}
		}
	case said != "" && !restartNoise.MatchString(said):
		return []sentence{{part: partProof, weight: weightError,
			text: errorVerb(f.lead.Entity) + " " + quoted(said) + "."}}
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

// usageSentences state how full something is: "It is at 94% and will
// be full in about 3 hours." or "etl uses about 11 GiB."
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
			text = subject + " is at " + used
		}
		if eta := evidence(s, "full in"); eta != "" {
			text += " and will be full in " + humanizeText(eta)
		}
		out = append(out, sentence{part: partProof, weight: weightUsage,
			text: endSentence(text)})
	}
	return out
}

// schedulerSentences say why the scheduler rejects every node: the
// investigated node counts, else the scheduler's own words.
func schedulerSentences(f caseFacts) []sentence {
	if counted := investigatedScheduler(f); counted != nil {
		return counted
	}
	for _, s := range f.members {
		if said := evidence(s, "scheduler"); said != "" {
			return []sentence{{part: partProof, weight: weightScheduler,
				text: "The scheduler says " + quoted(said) + "."}}
		}
	}
	return nil
}

// reasonConsequences say what the lead's own condition breaks.
var reasonConsequences = map[string]string{
	reasons.TLSCertExpiringSoon: "Clients will reject it once it expires.",
	reasons.TLSCertExpired:      "Clients are rejecting it now.",
	reasons.WebhookNoEndpoints: "It rejects every create and update " +
		"it checks.",
	reasons.WebhookBackendNotFound: "It rejects every create and " +
		"update it checks.",
}

// consequenceSentences say what the lead's condition breaks. A webhook
// only blocks requests when it is critical: its failurePolicy is Fail.
func consequenceSentences(f caseFacts) []sentence {
	text, ok := reasonConsequences[f.lead.Reason]
	if !f.ok || !ok {
		return nil
	}
	webhook := f.lead.Reason == reasons.WebhookNoEndpoints ||
		f.lead.Reason == reasons.WebhookBackendNotFound
	if webhook && f.lead.Severity != detection.Critical {
		return nil
	}
	return []sentence{{part: partConsequence, weight: 1, text: text}}
}

// restartNoise matches the kubelet's own restart message, which says
// nothing about why the container fails.
var restartNoise = regexp.MustCompile(
	`^(?i:back-off) \S+ restarting failed container`)

// pullNoise matches the kubelet's pull back-off, which only repeats the
// image name.
var pullNoise = regexp.MustCompile(`^(?i:back-off) pulling image `)

// errorVerb introduces a quoted error: a program "fails with" it, a
// node or service "reports" it.
func errorVerb(id inventory.EntityID) string {
	if id.Kind == kube.KindContainer || id.Kind == kube.KindPod {
		return "It fails with"
	}
	return "It reports"
}

// quoted puts a clipped error in quotes, without its final full stop.
func quoted(text string) string {
	return `"` + strings.TrimSuffix(clip(text), ".") + `"`
}

func lastLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// evidence is the first non-empty value under one of labels.
func evidence(s detection.Finding, labels ...string) string {
	for _, label := range labels {
		for _, e := range s.Evidence {
			if e.Label == label && strings.TrimSpace(e.Value) != "" {
				return strings.TrimSpace(e.Value)
			}
		}
	}
	return ""
}

// fieldWords names a changed field: "the image", or "key DB_HOST" for
// config data.
func fieldWords(path string) string {
	if key, ok := dataKey(path); ok {
		return "key " + key
	}
	return "the " + lastSegment(path)
}

// fieldNoun names a changed field without an article: "schedule", or
// "key DB_HOST".
func fieldNoun(path string) string {
	return strings.TrimPrefix(fieldWords(path), "the ")
}

// dataKey is the key of a config data path: "app.yaml" for
// "data.app.yaml". Keys may hold dots, so the whole rest is the key.
func dataKey(path string) (string, bool) {
	for _, prefix := range []string{"data.", "stringData.", "binaryData."} {
		if key, ok := strings.CutPrefix(path, prefix); ok {
			return key, true
		}
	}
	return "", false
}

// lastSegment is the last part of a field path: "image" for
// "spec.template.spec.containers[0].image".
func lastSegment(path string) string {
	if i := strings.LastIndexAny(path, ".]"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}
