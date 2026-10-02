package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// maxUpdate bounds an update: only what changed, in one or two
// sentences.
const maxUpdate = 2

// recoveredPrefix starts the timeline note of a member that recovered.
const recoveredPrefix = "recovered: "

// updateSentences say only what changed since the last message.
func updateSentences(f caseFacts) []sentence {
	switch f.reason {
	case incident.ReasonCauseRevised:
		return revisedSentences(f)
	case "flapping":
		return flappingSentences(f)
	}
	if changed := changedSentences(f); len(changed) > 0 {
		return changed
	}
	// Nothing specific is known to have changed: restate the case.
	return append(leadSentences(f), strongestProof(f)...)
}

// revisedSentences restate the case under its new cause, led by the
// subject: "api in shop has a revised cause: it was evicted because
// node n2 is low on memory". When the subject is the new cause itself,
// the lead says so: "Node n3 is the revised cause: it is low on
// memory".
func revisedSentences(f caseFacts) []sentence {
	lead := leadSentences(f)
	subject := leadSubject(f)
	name := f.leadName(subject)
	text := strings.TrimSuffix(leadText(f), ".")
	link := " has a revised cause: "
	if ownState(f) {
		link = " is the revised cause: "
	}
	lead[0].text = endSentence(capitalName(subject,
		name+link+"it "+withoutSubject(f, subject, text)))
	return append(lead, strongestProof(f)...)
}

// ownState reports a lead about the root's own condition, with no
// change or outside cause to blame: the root itself is the cause.
func ownState(f caseFacts) bool {
	p := f.p
	return !ownCause(p.Cause) && blamedChange(f) == nil &&
		(p.Cause == nil || rootFinding(p, f.members) != nil)
}

// withoutSubject is a lead without its subject: "payments in shop is
// down" and "payments is down in shop" both become "is down".
func withoutSubject(
	f caseFacts, subject inventory.EntityID, text string,
) string {
	if rest, ok := strings.CutPrefix(text, f.leadName(subject)+" "); ok {
		return rest
	}
	rest, ok := strings.CutPrefix(text, shortName(subject)+" ")
	if !ok {
		return stateWords(f.p)
	}
	if subject.Namespace != "" {
		rest = strings.Replace(rest,
			" in "+subject.Namespace+f.clusterTag(), "", 1)
	}
	return rest
}

func strongestProof(f caseFacts) []sentence {
	var proof []sentence
	for _, s := range writeAll(f, noteWriters) {
		if s.part == partProof {
			proof = append(proof, s)
		}
	}
	arranged := arrange(proof)
	return limitSentences(arranged, 1)
}

func flappingSentences(f caseFacts) []sentence {
	return []sentence{
		{part: partLead, text: capitalName(leadSubject(f),
			f.leadName(leadSubject(f))) +
			" keeps failing and recovering: it has recovered " +
			plural(len(f.p.Cycles), "time") + " recently."},
		{part: partRecurrence, text: "I'll write again when it stays " +
			"healthy or fails differently."},
	}
}

// changedSentences read the newest timeline entries: members that
// joined ("It's spreading to …") or recovered.
func changedSentences(f caseFacts) []sentence {
	current := map[string]detection.Finding{}
	for _, s := range f.members {
		current[describe(s.Entity)] = s
	}
	var out []sentence
	var spread []detection.Finding
	named := map[inventory.EntityID]bool{}
	for _, e := range newEvents(f.p) {
		text := e.Text
		recovered := strings.HasPrefix(text, recoveredPrefix)
		_, subject := splitSubject(strings.TrimPrefix(text, recoveredPrefix))
		s, active := current[subject]
		switch {
		case recovered && !active && subject != "":
			out = append(out, recoveredSentence(f, subject))
		case !recovered && active && !named[s.Entity]:
			named[s.Entity] = true
			spread = append(spread, s)
		}
	}
	out = append(out, spreadSentences(f, spread)...)
	return limitSentences(arrange(out), maxUpdate)
}

// spreadSentences say what newly fails, one sentence per condition,
// led by the incident's subject: "payments in shop is spreading to
// pods a and b: they have not been ready for three minutes." A cause
// that now shows its own condition is not spreading: "API service m
// now reports …".
func spreadSentences(f caseFacts, spread []detection.Finding) []sentence {
	home := leadSubject(f)
	var order []string
	byState := map[string][]inventory.EntityID{}
	var out []sentence
	// named records that a sentence already led with the subject, so
	// the spreading sentence says "It" instead of naming it again.
	named := false
	for _, s := range spread {
		state := predicate(s.Entity, s.Summary)
		if s.Entity == f.p.Root || (f.p.Cause != nil &&
			s.Entity == f.p.Cause.Root) {
			out = append(out, sentence{part: partLead,
				text: endSentence(capitalName(s.Entity,
					f.leadName(s.Entity)+" "+nowState(state)))})
			named = named || f.leadName(s.Entity) == f.leadName(home)
			continue
		}
		if _, seen := byState[state]; !seen {
			order = append(order, state)
		}
		byState[state] = append(byState[state], s.Entity)
	}
	for i, state := range order {
		ids := byState[state]
		text := capitalName(home, f.leadName(home)+" is spreading to ")
		switch {
		case i > 0:
			text = "It is also spreading to "
		case named:
			text = "It is spreading to "
		}
		text += nameList(home, ids)
		if len(ids) == 1 {
			text += ": it " + state
		} else {
			text += ": they " + pluralPredicate(state)
		}
		out = append(out, sentence{part: partLead, text: endSentence(text)})
	}
	return out
}

// recoveredSentence says a member recovered and what is still open,
// naming the subject: "Service cart has recovered; payments in shop
// still has one open failure."
func recoveredSentence(f caseFacts, subject string) sentence {
	id := parseDescribed(subject)
	home := leadSubject(f)
	failing := len(f.members)
	if id == home || failing == 0 {
		text := f.leadName(id) + " has recovered"
		if failing > 0 {
			text += "; " + plural(failing, "other failure") + " " +
				verb(failing, "is", "are") + " still open"
		}
		return sentence{part: partLead, text: capitalName(id, text+".")}
	}
	text := nameFrom(home, id) + " has recovered; " + f.leadName(home) +
		" still has " + plural(failing, "open failure")
	return sentence{part: partLead, text: capitalName(id, text+".")}
}

// parseDescribed reads back "kind namespace/name" as written by the
// incident timeline.
func parseDescribed(text string) inventory.EntityID {
	kind, rest, _ := strings.Cut(text, " ")
	namespace, name, found := strings.Cut(rest, "/")
	if !found {
		namespace, name = "", rest
	}
	return inventory.EntityID{Kind: inventory.Kind(kind),
		Namespace: namespace, Name: name}
}

// newEvents are the timeline entries the last delivered message did not
// report: every entry after the first Reported ones. Without that mark
// (after a restart) it falls back to the most recent evaluation.
func newEvents(p incident.Incident) []incident.Event {
	if !p.ReportedKnown {
		return latestEvents(p)
	}
	if p.Reported >= len(p.Timeline) {
		return nil
	}
	return p.Timeline[max(p.Reported, 0):]
}

// latestEvents are the timeline entries written in the most recent
// evaluation, which share its timestamp.
func latestEvents(p incident.Incident) []incident.Event {
	n := len(p.Timeline)
	if n == 0 {
		return nil
	}
	last := p.Timeline[n-1].At
	start := n - 1
	for start > 0 && p.Timeline[start-1].At.Equal(last) {
		start--
	}
	return p.Timeline[start:]
}

func limitSentences(sentences []sentence, n int) []sentence {
	if len(sentences) > n {
		return sentences[:n]
	}
	return sentences
}
