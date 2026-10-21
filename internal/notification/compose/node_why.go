package compose

import (
	"strings"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// unknownNodeText says what kwatch cannot know when the kubelet went
// silent: the node's own words are only "stopped posting". The plural is
// for an outage of several nodes.
const unknownNodeText = "kwatch cannot see why from inside the cluster: " +
	"the kubelet stopped reporting, so the node may be down or unreachable."

const unknownNodesText = "kwatch cannot see why from inside the cluster: " +
	"the kubelets stopped reporting, so the nodes may be down or " +
	"unreachable."

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
		text := unknownNodeText
		if leadIsNodes(f) {
			text = unknownNodesText
		}
		out = append(out, sentence{part: partUnverified, text: text})
	}
	return out
}

// nodeWhyText is the one proof sentence: what the node reports, also
// which conditions are True, and which events it logged. An outage of
// several nodes says "They report" and "Their".
func nodeWhyText(f caseFacts) string {
	it, its, reports := "It", "Its", "reports"
	if leadIsNodes(f) {
		it, its, reports = "They", "Their", "report"
	}
	text := ""
	if said := evidence(f.lead, "error", "message"); said != "" {
		text = it + " " + reports + " " + quoted(specificPart(said))
		if cert := explain.CertificateError(said); cert != "" {
			text = its + " kubelet" + pluralS(leadIsNodes(f)) + " " +
				reports + " " + quoted(cert)
		}
	}
	conditions := evidenceAll(f.lead, detection.EvidenceNodeCondition)
	events := evidenceAll(f.lead, detection.EvidenceNodeEvent)
	switch {
	case len(conditions) > 0 && text == "":
		text = it + " " + reports + " " + joinAnd(conditions, maxNamed)
	case len(conditions) > 0:
		text += " and also " + joinAnd(conditions, maxNamed)
	}
	switch {
	case len(events) > 0 && text == "":
		text = its + " recent events include " + joinAnd(events, maxNamed)
	case len(events) > 0:
		text += "; " + strings.ToLower(its) + " recent events include " +
			joinAnd(events, maxNamed)
	}
	return text
}

// pluralS is "s" for a plural noun.
func pluralS(many bool) string {
	if many {
		return "s"
	}
	return ""
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
