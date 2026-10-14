package compose

import (
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// weightAgentQuote ranks the agent's own words beside the event the
// node's pods report: both are proof, and neither hides the other.
const weightAgentQuote = 0.73

// agentQuoteSentences quote what a crash-looping pod of kube-system on
// the failing node last said. Such a pod is the node's agent (the CNI,
// the proxy, a storage plugin): its own error is the closest thing to
// the cause, and the node's message is the one place people read. The
// line is quoted as the container wrote it.
func agentQuoteSentences(f caseFacts) []sentence {
	if f.p.Root.Kind != kube.KindNode {
		return nil
	}
	for _, m := range failing(f.members) {
		if m.Entity.Kind != kube.KindContainer ||
			m.Entity.Namespace != "kube-system" {
			continue
		}
		line := evidence(m, "error")
		if line == "" || restartNoise.MatchString(line) {
			continue
		}
		pod, _ := splitContainer(m.Entity.Name)
		return []sentence{{part: partProof, weight: weightAgentQuote,
			text: "Pod " + pod + " in kube-system fails with " +
				quoted(line) + "."}}
	}
	return nil
}
