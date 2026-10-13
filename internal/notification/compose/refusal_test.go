package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuotaLineQuotesUsedHardAndRequested(t *testing.T) {
	got := quotaLine(`Error creating: pods "api-x" is forbidden: `+
		`exceeded quota: team-a, requested: requests.memory=1Gi, `+
		`used: requests.memory=31.5Gi, limited: requests.memory=32Gi`, "")

	assert.Equal(t, "ResourceQuota team-a: requests.memory 31.5Gi of "+
		"32Gi used; this pod asks 1Gi", got)
}

func TestQuotaLineListsEveryResourceTheCreateAsked(t *testing.T) {
	got := quotaLine(`exceeded quota: q, requested: cpu=2,pods=1, `+
		`used: cpu=7,pods=10, limited: cpu=8,pods=10`, "q")

	assert.Equal(t, "ResourceQuota q: cpu 7 of 8 used; this pod asks 2; "+
		"pods 10 of 10 used; this pod asks 1", got)
}

func TestQuotaLineIgnoresAnotherQuota(t *testing.T) {
	got := quotaLine(`exceeded quota: a, requested: pods=1, `+
		`used: pods=1, limited: pods=1`, "b")

	assert.Empty(t, got)
}

func TestLimitLineQuotesTheBoundAndTheAsk(t *testing.T) {
	assert.Equal(t, "LimitRange limits: max memory 1Gi per container; "+
		"a container asks 2Gi (limit)",
		limitLine("maximum memory usage per Container is 1Gi, "+
			"but limit is 2Gi", "limits"))
	assert.Equal(t, "LimitRange limits: min cpu 100m per container; "+
		"a container asks 50m (request)",
		limitLine("minimum cpu usage per Container is 100m, "+
			"but request is 50m, maximum cpu usage per Pod is 1",
			"limits"))
}

func TestLimitLineQuotesARatio(t *testing.T) {
	got := limitLine("memory max limit to request ratio per Container "+
		"is 2, but provided ratio is 4.000000", "")

	assert.Equal(t, "A LimitRange of the namespace: memory limit may be "+
		"at most 2x its request per container; a container has 4x", got)
}

func TestLimitLineSaysNothingForOtherText(t *testing.T) {
	assert.Empty(t, limitLine("pods is forbidden: no", "x"))
}
