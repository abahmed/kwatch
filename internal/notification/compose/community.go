package compose

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// This file is the Community edition of a message. The engine still
// finds the cause, the changes and the impact; a Community message says
// what is failing and only the category of its cause ("Cause found: a
// config change.", see cause_category.go), and drops the detail: the
// cause's object, change, version, image and wording, what to do (the
// steps, the command to run and the setting to change), proofs,
// counterfactual checks, confidence, the recent change, who made a
// change, how often it happened before, affected services and
// investigation output. Lists (digests, roll-ups, grouped recoveries)
// use the same Community lead.

// ForEdition returns a writer for the edition e entitles. An edition
// without root cause detail writes the Community message; its Pro note
// follows e.Limits().ProHints unless hideHints is set. Without Impact
// no sentence says what else the failure affects. A nil e is the
// Community edition.
func (w Writer) ForEdition(e feature.Entitlements, hideHints bool) Writer {
	e = feature.OrCommunity(e)
	w.NoImpact = !e.Has(feature.Impact)
	if e.Limits().RootCauseDetail {
		return w
	}
	w.Community = true
	w.ProNote = e.Limits().ProHints && !hideHints
	return w
}

// listWriter writes the title a list (a digest, a roll-up, a grouped
// recovery) gives an incident: the edition's lead, without the cluster
// tag, links or Pro note, so a Community list never tells a cause the
// incident's own Community message leaves out.
func (w Writer) listWriter() Writer {
	return Writer{Community: w.Community}
}

// communityParts are the parts of a note that carry detail Pro adds.
// The consequences go with Impact (NoImpact), not with this list.
var communityParts = map[part]bool{
	partProof: true, partChanges: true, partChecked: true,
	partAction: true, partPlace: true, partRecurrence: true,
}

// proLine is the Pro note of a Community message. cause is the line of
// a message that tells its cause's category; plain is the line of one
// that does not (an update that only says what changed).
type proLine struct {
	plain, cause string
}

// communityOf is d as a Community message tells it, and the message's
// Pro note.
func (w Writer) communityOf(
	d incident.Decision, now time.Time,
) (incident.Decision, proLine) {
	plain := communityDecision(d)
	note := proLine{plain: w.noteFor(d, plain, now)}
	if w.ProNote && d.Action != incident.Resolve {
		note.cause = causeNote(plain.Incident.Cause,
			fixable(gatherFacts(plain, now, nil)))
	}
	return plain, note
}

// noteFor is the Pro note of d, when this writer adds one and the
// decision is a message that can carry it. plain is d as the Community
// message tells it: the fix it leaves out is what the note names.
func (w Writer) noteFor(
	d, plain incident.Decision, now time.Time,
) string {
	if !w.ProNote || d.Action == incident.Resolve {
		return ""
	}
	return proNote(d, fixable(gatherFacts(plain, now, nil)))
}

// fixable reports a message whose Pro edition says how to fix it: steps
// to run or the setting to change.
func fixable(f caseFacts) bool {
	return len(proSteps(f)) > 0 || len(placeSentences(f)) > 0
}

// withoutPart is sentences without those of part p.
func withoutPart(sentences []sentence, p part) []sentence {
	var out []sentence
	for _, s := range sentences {
		if s.part != p {
			out = append(out, s)
		}
	}
	return out
}

// communityDecision is d without the detail a Community message omits.
// The decision is copied: the incident keeps its cause, and /status and
// the audit log read it whole. The ties to other incidents go too: they
// come from the other incident's failure chain, which is Pro detail, so
// a Community message says neither "Caused by" nor "Likely caused by"
// and the lines that say no cause is known speak again. Who made a
// change is Pro detail as well, so the changes the incident still
// carries lose their actor here, once for every sentence.
func communityDecision(d incident.Decision) incident.Decision {
	d.Facts.Output, d.Facts.Evidence = nil, nil
	d.Facts.Report, d.Facts.Correlation = nil, nil
	d.Facts.Changes, d.Facts.Window, d.Facts.Wake = nil, nil, nil
	d.Incident.Cause = communityCause(d.Incident.Cause)
	d.Incident.CausedBy, d.Incident.Causing = nil, nil
	d.Incident.Attempt = withoutActor(d.Incident.Attempt)
	d.Incident.FixedBy = withoutActor(d.Incident.FixedBy)
	return d
}

// withoutActor is a copy of c that does not say who made it.
func withoutActor(c *inventory.Change) *inventory.Change {
	if c == nil {
		return nil
	}
	plain := *c
	plain.Actor = ""
	return &plain
}

// communityCause keeps the cause's row, mode and score, from which the
// message tells its category, and drops its proof, checks, blamed
// change, failure chain and close rival, so a Community message tells
// one cause, never "two possible causes".
func communityCause(c *rootcause.CauseRecord) *rootcause.CauseRecord {
	if c == nil {
		return nil
	}
	plain := *c
	plain.Proof, plain.Checked, plain.Change = nil, nil, nil
	plain.RollbackRevision = ""
	plain.Hops, plain.Beyond = nil, nil
	plain.Rival = nil
	return &plain
}

// communitySentences drops the sentences that carry Pro detail and, for
// a first message or an update, adds the Pro note. A message that tells
// its cause's category drops every other sentence about the cause too:
// they name its object.
func (w Writer) communitySentences(
	sentences []sentence, note proLine,
) []sentence {
	teased := slices.ContainsFunc(sentences, func(s sentence) bool {
		return s.fact == factCauseTeaser
	})
	var out []sentence
	for _, s := range sentences {
		if !communityParts[s.part] && (!teased || !aboutCause(s)) {
			out = append(out, s)
		}
	}
	line := note.plain
	if teased {
		line = note.cause
	}
	if line != "" && len(out) > 0 {
		out = append(out, sentence{part: partHint, text: line})
	}
	return out
}

// aboutCause reports a sentence that speaks of the cause, other than
// the one that tells its category.
func aboutCause(s sentence) bool {
	return s.fact != factCauseTeaser &&
		(s.part == partCause || s.part == partUnverified)
}
