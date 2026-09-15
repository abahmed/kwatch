package startup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSummaryAggregatesByReason(t *testing.T) {
	inc := BuildSummary(true, map[string]int{
		"arc-system/webhook/ServiceNoEndpoints":   1,
		"arc-system/metrics/ServiceNoEndpoints":   1,
		"istio-system/istiod/ServicePortMismatch": 1,
		"kwatch/OOMKilled":                        1,
	})

	assert.NotNil(t, inc)
	assert.Equal(t, 4, inc.Count)
	assert.Contains(t, inc.Hint, "4 pre-existing issue(s)")
	assert.Contains(t, inc.Hint, "ServiceNoEndpoints ×2")
	assert.Contains(t, inc.Hint, "across 4 workloads")
	assert.NotContains(t, inc.Hint, "arc-system/webhook")
	assert.Less(
		t,
		indexOf(inc.Hint, "ServiceNoEndpoints"),
		indexOf(inc.Hint, "OOMKilled"),
	)
}

func TestBuildSummaryCapsReasonKinds(t *testing.T) {
	suppressed := map[string]int{}
	for _, reason := range []string{
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J",
	} {
		suppressed["ns/w/"+reason] = 1
	}
	inc := BuildSummary(true, suppressed)
	assert.Contains(t, inc.Hint, "+2 other kinds")
}

func TestBuildSummaryKeepsBareReasonKeysWithoutWorkloads(t *testing.T) {
	inc := BuildSummary(true, map[string]int{"CrashLoopBackOff": 3})
	assert.Equal(t, 3, inc.Count)
	assert.Contains(t, inc.Hint, "CrashLoopBackOff ×3")
	assert.NotContains(t, inc.Hint, "workloads")
}

func TestBuildSummaryDisabledOrEmpty(t *testing.T) {
	assert.Nil(t, BuildSummary(false, map[string]int{"Reason": 1}))
	assert.Nil(t, BuildSummary(true, nil))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
