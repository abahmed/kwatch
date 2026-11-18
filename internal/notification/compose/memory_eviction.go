package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// weightEviction ranks the node's memory behind a memory eviction as
// direct proof, like the node's memory behind a node OOM kill.
const weightEviction = 0.7

// evictionSentences say, for a pod evicted while its node was short of
// memory, what the pod used against its request and who used the most:
// "It used 900Mi, over its 256Mi request. Its node was short of memory
// (requested 7.5Gi of 8Gi allocatable); biggest users: batch/importer
// 3.1Gi (request 1Gi, no limit)." Nothing is said for any other
// eviction.
func evictionSentences(f caseFacts) []sentence {
	if f.p.Root.Kind == kube.KindNode {
		// The node is the subject: its own pressure says it.
		return nil
	}
	for _, m := range f.members {
		if m.Reason != reasons.Evicted ||
			evidence(m, detection.EvidenceOOMKind) == "" {
			continue
		}
		var out []sentence
		if use := evidence(m, detection.EvidenceOverRequest); use != "" {
			out = append(out, sentence{part: partProof,
				weight: weightEviction + 0.01,
				text:   endSentence("It " + use)})
		}
		text := "Its node was short of memory"
		if node := evidence(m, detection.EvidenceNodeMemory); node != "" {
			text += " (" + node + ")"
		}
		if users := nodeUsers(m); len(users) > 0 {
			text += "; biggest users: " + joinAnd(users, maxNamed)
		}
		return append(out, sentence{part: partProof,
			weight: weightEviction, text: endSentence(text)})
	}
	return nil
}
