package compose

import (
	"strings"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// unknownNodeText says what kwatch cannot know when the kubelet went
// silent: the node's own words are only "stopped posting".
const unknownNodeText = "kwatch cannot see why from inside the cluster: " +
	"the kubelet stopped reporting, so the node may be down or unreachable."

// nodeLead reports a case led by one node that is NotReady or silent.
func nodeLead(f caseFacts) bool {
	return f.ok && f.lead.Reason == reasons.NodeNotReady && !leadIsGroup(f)
}

// nodeWhySentences say why a node is NotReady in its own words: the
// Ready message, the other conditions that are True and its recent
// Warning events, as the detector quoted them. A node that stopped
// reporting says nothing more, and the note says so.
func nodeWhySentences(f caseFacts) []sentence {
	if !nodeLead(f) {
		return nil
	}
	var out []sentence
	if text := nodeWhyText(f); text != "" {
		out = append(out, sentence{part: partProof, weight: weightError,
			text: endSentence(text)})
	}
	if evidence(f.lead, detection.EvidenceReadyStatus) == "Unknown" {
		out = append(out, sentence{part: partUnverified,
			text: unknownNodeText})
	}
	return out
}

// nodeWhyText is the one proof sentence: what the node reports, also
// which conditions are True, and which events it logged.
func nodeWhyText(f caseFacts) string {
	text := ""
	if said := evidence(f.lead, "error", "message"); said != "" {
		text = "It reports " + quoted(specificPart(said))
	}
	conditions := evidenceAll(f.lead, detection.EvidenceNodeCondition)
	events := evidenceAll(f.lead, detection.EvidenceNodeEvent)
	switch {
	case len(conditions) > 0 && text == "":
		text = "It reports " + joinAnd(conditions, maxNamed)
	case len(conditions) > 0:
		text += " and also " + joinAnd(conditions, maxNamed)
	}
	switch {
	case len(events) > 0 && text == "":
		text = "Its recent events include " + joinAnd(events, maxNamed)
	case len(events) > 0:
		text += "; its recent events include " + joinAnd(events, maxNamed)
	}
	return text
}

// evidenceAll is every non-empty value under one label.
func evidenceAll(s detection.Finding, label string) []string {
	var out []string
	for _, e := range s.Evidence {
		if v := strings.TrimSpace(e.Value); e.Label == label && v != "" {
			out = append(out, v)
		}
	}
	return out
}

// specificPart keeps the line of a long kubelet message that says what
// is wrong. The runtime's network message nests its cause at the end
// ("... message:Network plugin returns error: cni plugin not
// initialized"): the start names the check, the end names the fault,
// and the middle is cut.
func specificPart(said string) string {
	said = strings.TrimSpace(said)
	if utf8.RuneCountInString(said) <= maxQuote {
		return said
	}
	head, _, ok := strings.Cut(said, ": ")
	last := strings.LastIndex(said, "message:")
	if !ok || last < 0 {
		return said
	}
	tail := strings.TrimSpace(said[last+len("message:"):])
	return head + ": … " + tail
}

// hasProof reports whether any of the sentences is proof.
func hasProof(list []sentence) bool {
	for _, s := range list {
		if s.part == partProof {
			return true
		}
	}
	return false
}
