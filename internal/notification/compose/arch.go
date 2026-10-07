package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
)

// weightArchHealthy ranks the healthy replicas high: they are what turns
// "exec format error" from a guess into a finding.
const weightArchHealthy = 0.72

// archSentences say that the same workload runs fine on nodes of another
// CPU architecture. The detector wrote the count; the crash and its
// quoted error line are said elsewhere.
func archSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		healthy := evidence(m, detection.EvidenceArchHealthy)
		if healthy == "" {
			continue
		}
		return []sentence{{part: partProof, weight: weightArchHealthy,
			text: "The " + healthy + " run fine."}}
	}
	return nil
}
