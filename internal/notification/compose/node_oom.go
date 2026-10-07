package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/notification"
)

// Node OOM sentences rank above the plain memory history: when the node
// ran out of memory, "killed at its limit" would be wrong.
const (
	weightNodeOOMUse  = 0.72
	weightNodeOOMNode = 0.7
)

// nodeKill is the OOM-killed member the detector says the node killed,
// and the node it names. ok is false for a kill by the container's own
// limit.
func nodeKill(members []detection.Finding) (detection.Finding, string, bool) {
	for _, m := range members {
		if m.Reason != reasons.OOMKilled {
			continue
		}
		if node := evidence(m, detection.EvidenceKilledByNode); node != "" {
			return m, node, true
		}
	}
	return detection.Finding{}, "", false
}

// nodeOOMSentences say what the victim used against its limit, and which
// node ran out of memory and who used most of it. The numbers come from
// the finding's evidence; nothing is guessed here.
func nodeOOMSentences(f caseFacts) []sentence {
	m, node, ok := nodeKill(f.members)
	if !ok {
		return nil
	}
	var out []sentence
	used := evidence(m, detection.EvidenceMemoryUsed)
	limit := evidence(m, detection.EvidenceMemoryLimit)
	switch {
	case used != "" && limit != "":
		out = append(out, sentence{part: partProof, weight: weightNodeOOMUse,
			text: "It used " + used + " of its " + limit + " limit."})
	case used != "":
		out = append(out, sentence{part: partProof, weight: weightNodeOOMUse,
			text: "It used " + used + " and has no memory limit."})
	}
	text := "Node " + node + " ran out of memory"
	if users := nodeUsers(m); len(users) > 0 {
		text += "; biggest users: " + joinAnd(users, maxNamed)
	}
	return append(out, sentence{part: partProof, weight: weightNodeOOMNode,
		text: endSentence(text)})
}

// nodeUsers lists the node's biggest memory users the finding names.
func nodeUsers(m detection.Finding) []string {
	var out []string
	for _, e := range m.Evidence {
		value := strings.TrimSpace(e.Value)
		if e.Label == detection.EvidenceNodeMemoryUser && value != "" {
			out = append(out, value)
		}
	}
	return out
}

// nodeOOMSteps send the reader to the node, not to the victim's limit.
func nodeOOMSteps(s detection.Finding) []notification.Step {
	node := evidence(s, detection.EvidenceKilledByNode)
	if s.Reason != reasons.OOMKilled || node == "" {
		return nil
	}
	return []notification.Step{{
		Text:    "See the node's memory use and the pods on it",
		Command: "kubectl describe node " + quote(node),
	}}
}
