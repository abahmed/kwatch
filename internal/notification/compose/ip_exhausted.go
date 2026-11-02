package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// weightIPExhausted ranks the count and the quote above generic proof:
// they are the whole story of this failure.
const weightIPExhausted = 0.74

// ipTally adds up the node findings of an incident whose nodes ran out
// of pod addresses.
type ipTally struct {
	// failed counts the pods that failed to get an address within the
	// event window; stuck the ones of them that have not started since.
	failed, stuck, nodes int
	// stuckKnown is false for a finding without the stuck count.
	stuckKnown bool
	node       inventory.EntityID
	line       string
}

// ipExhaustedSentences count the pods that cannot start on the nodes
// that ran out of pod addresses, and quote what their events say. The
// detector counted the pods and kept the newest event message; nothing
// is reworded here. When the incident's root is the one node, the lead
// already names it: the count does not say "on 1 node" again.
func ipExhaustedSentences(f caseFacts) []sentence {
	t := tallyIPExhausted(f.members)
	if t.failed == 0 {
		return nil
	}
	text := ipCountText(t, t.nodes == 1 && t.node == f.p.Root)
	if t.line != "" {
		text += " Their events say " + quoted(t.line) + "."
	}
	return []sentence{{part: partProof, weight: weightIPExhausted,
		text: text}}
}

func tallyIPExhausted(members []detection.Finding) ipTally {
	t := ipTally{stuckKnown: true}
	for _, m := range members {
		if m.Reason != reasons.NodePodIPExhausted {
			continue
		}
		n, err := strconv.Atoi(evidence(m, "affected pods"))
		if err != nil || n < 1 {
			continue
		}
		t.failed += n
		t.nodes++
		t.node = m.Entity
		p, err := strconv.Atoi(evidence(m, detection.EvidenceStuckPods))
		if err != nil {
			t.stuckKnown = false
		}
		t.stuck += p
		if t.line == "" {
			t.line = evidence(m, detection.EvidenceSandboxEvent)
		}
	}
	return t
}

// ipCountText says how many pods can't start: "42 pods can't start on 3
// nodes: no free pod IPs." The pods that failed to get an address and
// started since are not stuck; they are said apart, so the count is
// the pods that cannot start now.
func ipCountText(t ipTally, rootNode bool) string {
	place := " on " + strconv.Itoa(t.nodes) + " " + nodeNoun(t.nodes)
	if rootNode {
		place = ""
	}
	if !t.stuckKnown || t.stuck == t.failed {
		return podCount(t.failed) + " can't start" + place +
			": no free pod IPs."
	}
	if t.stuck == 0 {
		return podCount(t.failed) + place + " failed to get a pod " +
			"IP recently; all have started since."
	}
	return podCount(t.stuck) + " can't start" + place +
		": no free pod IPs; " + strconv.Itoa(t.failed-t.stuck) +
		" more failed to get one recently and have started since."
}

func podCount(n int) string {
	if n == 1 {
		return "1 pod"
	}
	return strconv.Itoa(n) + " pods"
}

func nodeNoun(n int) string {
	if n == 1 {
		return "node"
	}
	return "nodes"
}
