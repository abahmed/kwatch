package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightIPExhausted ranks the count and the quote above generic proof:
// they are the whole story of this failure.
const weightIPExhausted = 0.74

// ipExhaustedSentences count the pods that cannot start on the nodes
// that ran out of pod addresses, and quote what their events say. The
// detector counted the pods and kept the newest event message; nothing
// is reworded here.
func ipExhaustedSentences(f caseFacts) []sentence {
	pods, nodes, line := 0, 0, ""
	for _, m := range f.members {
		if m.Reason != reasons.NodePodIPExhausted {
			continue
		}
		n, err := strconv.Atoi(evidence(m, "affected pods"))
		if err != nil || n < 1 {
			continue
		}
		pods += n
		nodes++
		if line == "" {
			line = evidence(m, detection.EvidenceSandboxEvent)
		}
	}
	if pods == 0 {
		return nil
	}
	text := strconv.Itoa(pods) + " pods can't start on " +
		strconv.Itoa(nodes) + " " + nodeNoun(nodes) +
		": no free pod IPs."
	if line != "" {
		text += " Their events say " + quoted(line) + "."
	}
	return []sentence{{part: partProof, weight: weightIPExhausted,
		text: text}}
}

func nodeNoun(n int) string {
	if n == 1 {
		return "node"
	}
	return "nodes"
}
