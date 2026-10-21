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

// updateSentences say only what changed since the last message.
func updateSentences(f caseFacts) []sentence {
	build, _ := sentencesFor(f.reason)
	return build(f)
}

// sentencesFor picks the sentence builder of a decision reason. known is
// false for a reason compose has no rule for (such as a free-text resolve
// reason); the builder is then the material-change one. Every value in
// incident.AllReasons must be known (see TestEveryReasonIsHandled).
func sentencesFor(
	reason incident.Reason,
) (build func(caseFacts) []sentence, known bool) {
	switch reason {
	case incident.ReasonCauseRevised:
		return revisedSentences, true
	case incident.ReasonFlapping:
		return flappingSentences, true
	case incident.ReasonReminder:
		return reminderSentences, true
	case incident.ReasonFixAttempt:
		return attemptSentences, true
	case incident.ReasonFixStillFailing:
		return stillFailingSentences, true
	case incident.ReasonFailingAgain:
		return failingAgainSentences, true
	case incident.ReasonAcknowledged:
		return acknowledgedSentences, true
	case incident.ReasonAckRemoved:
		return ackRemovedSentences, true
	case incident.ReasonMaterialChange:
		return changeSentencesFor, true
	case incident.ReasonSettled, incident.ReasonSuperseded:
		// Announce and Resolve decisions never reach updateSentences.
		return changeSentencesFor, true
	}
	return changeSentencesFor, false
}

// changeSentencesFor says what changed: worsening, a rollout, a changed
// fact, or else the case restated.
func changeSentencesFor(f caseFacts) []sentence {
	if worse := worseningSentences(f); len(worse) > 0 {
		return worse
	}
	if text, ok := firstRolloutLead(f); ok {
		return []sentence{{part: partLead, text: capitalName(
			leadSubject(f), endSentence(text))}}
	}
	if changed := changedSentences(f); len(changed) > 0 {
		return changed
	}
	// Nothing specific is known to have changed: restate the case, and
	// say what is news when the tier rose.
	out := append(leadSentences(f), raisedSentences(f)...)
	return append(out, strongestProof(f)...)
}

// raisedSentences say that an incident got louder since the last
// message: "It is now critical." Without a rise they say nothing, so a
// restated case never claims news it does not have.
func raisedSentences(f caseFacts) []sentence {
	if !f.p.Delivery.TierRaised() {
		return nil
	}
	words := map[incident.Tier]string{incident.Page: "critical",
		incident.Notify: "a warning"}
	word, ok := words[f.p.Tier]
	if !ok {
		return nil
	}
	return []sentence{{part: partLead, text: "It is now " + word + "."}}
}

// revisedSentences restate the case under its new cause, led by what
// the thread now learns: "Cause now known: Node n3 is low on memory."
// A cause that replaces one already told says "Cause changed:" instead.
func revisedSentences(f caseFacts) []sentence {
	lead := leadSentences(f)
	label := "Cause now known: "
	if f.p.Pending.CauseReplaced() {
		label = "Cause changed: "
	}
	lead[0].text = label + lead[0].text
	lead = append(lead, strongestProof(f)...)
	return append(lead, formerRootSentences(f)...)
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
		{part: partRecurrence, text: "kwatch will write again when it stays " +
			"healthy or fails differently."},
	}
}

// changedSentences read the newest timeline entries: members that
// joined ("It's spreading to …") or recovered.
func changedSentences(f caseFacts) []sentence {
	current := map[inventory.EntityID]detection.Finding{}
	for _, s := range failing(f.members) {
		current[s.Entity] = s
	}
	var spread []detection.Finding
	var ended []inventory.EntityID
	named := map[inventory.EntityID]bool{}
	for _, e := range newEvents(f.p) {
		if e.Entity == nil {
			continue
		}
		recovered := strings.HasPrefix(e.Text, incident.RecoveredPrefix)
		s, active := current[*e.Entity]
		switch {
		case recovered && !active:
			ended = append(ended, *e.Entity)
		case !recovered && active && !named[s.Entity]:
			named[s.Entity] = true
			spread = append(spread, s)
		}
	}
	out := recoveredSentences(f, ended)
	out = append(out, spreadSentences(f, spread)...)
	return limitSentences(arrange(out), maxUpdate)
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
