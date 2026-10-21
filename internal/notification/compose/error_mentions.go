package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// mentionWithError puts the error a cause's "the error mentions it"
// proof is about in front of it: "It fails with "...". The error
// mentions it." A reader cannot check a claim about an error the note
// does not show, so without an error to quote the proof is dropped. When
// only some of the errors name the cause, the quoted one must be one of
// them.
func mentionWithError(
	f caseFacts, item rootcause.Proof, mention string,
) (sentence, bool) {
	quote := errorSentences(f)
	if len(quote) == 0 {
		return sentence{}, false
	}
	if item.Count < item.Total && !errorNamesCause(f) {
		return sentence{}, false
	}
	// It outranks the plain error sentence it contains, which the note
	// then drops as a repeat of the same quotation.
	return sentence{part: partProof,
		weight: max(item.Weight, quote[0].weight) + 0.01,
		text:   quote[0].text + " " + mention}, true
}

// errorNamesCause reports that the lead's own error text names the
// cause's root.
func errorNamesCause(f caseFacts) bool {
	if f.p.Cause == nil || !f.ok {
		return false
	}
	said := evidence(f.lead, detection.EvidenceError, "message")
	name := strings.ToLower(f.p.Cause.Root.Name)
	return name != "" && strings.Contains(strings.ToLower(said), name)
}
