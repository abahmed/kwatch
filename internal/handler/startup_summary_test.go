package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Three hundred "ns/name/Reason ×1" entries cut off by the chat provider said
// "a lot" and nothing else; the summary groups by kind and counts workloads.
func TestStartupSummaryHintAggregatesByReason(t *testing.T) {
	hint, total := startupSummaryHint(map[string]int{
		"arc-system/webhook/ServiceNoEndpoints":   1,
		"arc-system/metrics/ServiceNoEndpoints":   1,
		"istio-system/istiod/ServicePortMismatch": 1,
		"kwatch/OOMKilled":                        1,
	})

	assert.Equal(t, 4, total)
	assert.Contains(t, hint, "4 pre-existing issue(s)")
	assert.Contains(t, hint, "ServiceNoEndpoints ×2")
	assert.Contains(t, hint, "across 4 workloads")
	assert.NotContains(t, hint, "arc-system/webhook",
		"individual workloads are counted, not listed")
	// Most frequent kind first.
	assert.Less(t,
		indexOf(hint, "ServiceNoEndpoints"), indexOf(hint, "OOMKilled"))
}

func TestStartupSummaryHintCapsReasonKinds(t *testing.T) {
	suppressed := map[string]int{}
	for _, r := range []string{
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J",
	} {
		suppressed["ns/w/"+r] = 1
	}
	hint, _ := startupSummaryHint(suppressed)
	assert.Contains(t, hint, "+2 other kinds")
}

func TestStartupSummaryHintBareReasonKeys(t *testing.T) {
	hint, total := startupSummaryHint(map[string]int{"CrashLoopBackOff": 3})
	assert.Equal(t, 3, total)
	assert.Contains(t, hint, "CrashLoopBackOff ×3")
	assert.NotContains(t, hint, "workloads")
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
