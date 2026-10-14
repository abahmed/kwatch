package compose

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// weightWhyNow ranks the joined sentence beside the other consequences.
const weightWhyNow = 0.45

// whySentences join, in one factual sentence, what the failing
// container's setting allowed and used with when the failure began
// against the change before it: "It was killed at its 256Mi memory
// limit; it used 255Mi at peak in the last 24 hours; it has failed
// since 10:05, 20s after the 10:05 release (revision 14)." They need a
// real failure, a measurement and a change that came first. The
// sentence that said the measurement alone stays silent when this one
// carries it (see sentence.carries).
func whySentences(f caseFacts) []sentence {
	text, kind := f.why()
	if text == "" {
		return nil
	}
	if kind == whyLiveness && f.p.Cause != nil &&
		livenessStartRules[f.p.Cause.Rule] {
		// The sentence that said the numbers also said what they mean
		// and what fixes it; carrying the numbers must not lose that.
		text += " " + upperFirst(livenessStartRemedy)
	}
	return []sentence{{part: partConsequence, weight: weightWhyNow,
		text: text, carries: kind.facts()}}
}

// whyKind says which measurement a joined sentence carries, so the
// sentence that used to say it alone can stay silent.
type whyKind uint8

const (
	whyNone whyKind = iota
	whyMemory
	whyLiveness
)

// facts are the plainer facts the joined sentence of this kind carries.
func (k whyKind) facts() []factKey {
	switch k {
	case whyMemory:
		return []factKey{factMemory}
	case whyLiveness:
		return []factKey{factLiveness}
	}
	return nil
}

// why builds the joined sentence; "" when there is none.
func (f caseFacts) why() (string, whyKind) {
	if onlyAdvisory(f.members) {
		return "", whyNone
	}
	measure, kind := f.measurement()
	began := f.failureStart()
	change, ok := f.triggerChange(began)
	if measure == "" || !ok {
		return "", whyNone
	}
	text := measure + "; it has failed since " + clock(began) + ", " +
		format.Duration(began.Sub(change.At)) + " after the " +
		clock(change.At) + " " + triggerNoun(change) +
		triggerRevision(change)
	return endSentence(upperFirst(text)), kind
}

// measurement is what a failing container's setting allowed and what
// it needed, from the numbers its finding holds; "" when no failure
// has them. Memory first, then liveness, then CPU.
func (f caseFacts) measurement() (string, whyKind) {
	for _, m := range failing(f.members) {
		switch {
		case m.Reason == reasons.OOMKilled &&
			evidence(m, detection.EvidenceKilledByNode) == "":
			if text := memoryMeasure(m); text != "" {
				return text, whyMemory
			}
		case m.Reason == reasons.LivenessKilled:
			if text := livenessMeasure(m); text != "" {
				return text, whyLiveness
			}
		}
	}
	return "", whyNone
}

// memoryMeasure is the memory fact the kill shows, as a clause: "it
// was killed at its 512Mi memory limit; it used 470Mi at peak in the
// last 24 hours".
func memoryMeasure(m detection.Finding) string {
	text := strings.TrimSuffix(memoryText(m), ".")
	if text == "" {
		return ""
	}
	return lowerFirst(text)
}

// livenessMeasure is "liveness allows 40s; it normally needs about 75s
// to become ready (last 5 starts)".
func livenessMeasure(m detection.Finding) string {
	gives := evidence(m, detection.EvidenceLivenessGives)
	usual := evidence(m, detection.EvidenceUsualStart)
	if gives == "" || usual == "" {
		return ""
	}
	text := "liveness allows " + gives + "; it normally needs about " +
		usual + " to become ready"
	if n := evidence(m, detection.EvidenceStartSamples); n != "" {
		text += " (last " + n + " starts)"
	}
	return text
}

// failureStart is when the earliest failing member began to fail; the
// zero time when none says.
func (f caseFacts) failureStart() time.Time {
	var first time.Time
	for _, m := range failing(f.members) {
		if !m.Since.IsZero() && (first.IsZero() || m.Since.Before(first)) {
			first = m.Since
		}
	}
	return first
}

// triggerChange is the change to the incident's own object that came
// just before the failure began: the blamed change, else the root's
// latest own change inside the causal window.
func (f caseFacts) triggerChange(began time.Time) (inventory.Change, bool) {
	if began.IsZero() {
		return inventory.Change{}, false
	}
	candidates := append([]inventory.Change(nil), f.changes...)
	if f.p.Cause != nil && f.p.Cause.Change != nil {
		candidates = append([]inventory.Change{*f.p.Cause.Change},
			candidates...)
	}
	for _, change := range candidates {
		gap := began.Sub(change.At)
		if change.Entity == f.p.Root && !systemChange(change) &&
			!change.Created && !change.Deleted &&
			gap >= 0 && gap <= explain.DefaultWindow {
			return change, true
		}
	}
	return inventory.Change{}, false
}

// triggerNoun is "release" for a change to what the pods run (a
// container field or a revision), else "change".
func triggerNoun(change inventory.Change) string {
	if change.Revision != "" {
		return "release"
	}
	for _, field := range change.Fields {
		if strings.HasPrefix(field.Path, "containers[") {
			return "release"
		}
	}
	return "change"
}

// triggerRevision is " (revision 14)" when the change has a numeric
// revision; other revision labels ("generation 2") are left out.
func triggerRevision(change inventory.Change) string {
	if _, err := strconv.Atoi(change.Revision); err != nil {
		return ""
	}
	return " (revision " + change.Revision + ")"
}
