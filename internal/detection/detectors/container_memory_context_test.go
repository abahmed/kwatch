package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestOOMByOwnLimitStatesHistoryAndBounds(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 500 * mib, limit: 512 * mib,
		more: map[string]inventory.Value{
			kube.AttrMemoryReq: inventory.Number(128 * mib),
			kube.AttrMemoryHistorySince: inventory.Time(
				podNodeNow.Add(-5 * time.Hour)),
			kube.AttrMemoryPeakHours: inventory.Number(4),
			kube.AttrMemoryRunsEnded: inventory.Number(3),
			kube.AttrMemoryRunsLow:   inventory.Number(495 * mib),
			kube.AttrMemoryRunsHigh:  inventory.Number(510 * mib),
		}})

	assert.Equal(t, "its own memory limit",
		evidenceOf(f, detection.EvidenceOOMKind))
	assert.Equal(t, "128Mi", evidenceOf(f, detection.EvidenceMemoryRequest))
	assert.Equal(t, "5h", evidenceOf(f, detection.EvidenceMemoryWindow))
	assert.Equal(t, "4", evidenceOf(f, detection.EvidenceMemoryNearLimit))
	assert.Equal(t, "3 runs, each near 510Mi",
		evidenceOf(f, detection.EvidenceMemoryRuns))
	assert.Equal(t, "16Gi", evidenceOf(f, detection.EvidenceMemoryNodeMax))
	assert.Empty(t, evidenceOf(f, detection.EvidenceBoundsUnread),
		"every kind is readable in the test context")
}

func TestOOMWithoutHistorySaysSoAndInventsNothing(t *testing.T) {
	f := detectOOM(t, oomFixture{limit: 512 * mib})

	assert.Equal(t, "none before the kill",
		evidenceOf(f, detection.EvidenceMemoryHistory))
	for _, label := range []string{detection.EvidenceMemoryWindow,
		detection.EvidenceMemoryNearLimit, detection.EvidenceMemoryRuns,
		detection.EvidenceMemoryFlat, detection.EvidenceMemoryPeak} {
		assert.Empty(t, evidenceOf(f, label), label)
	}
}

func TestOOMPeakBelowLimitHasNoNearLimitHours(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 300 * mib, limit: 512 * mib,
		more: map[string]inventory.Value{
			kube.AttrMemoryPeakHours: inventory.Number(4),
		}})
	assert.Empty(t, evidenceOf(f, detection.EvidenceMemoryNearLimit),
		"hours near a peak below the limit are not hours at the limit")
}

func TestNodeKillStatesNodeMemoryAndRequest(t *testing.T) {
	f := detectOOM(t, oomFixture{peak: 180 * mib, limit: 512 * mib,
		pressure: true, more: map[string]inventory.Value{
			kube.AttrMemoryReq: inventory.Number(128 * mib),
		}})

	assert.Equal(t, "the node's memory",
		evidenceOf(f, detection.EvidenceOOMKind))
	assert.Equal(t, "requested 128Mi of 16Gi allocatable; 3.3Gi in use",
		evidenceOf(f, detection.EvidenceNodeMemory))
	assert.Equal(t, "used 180Mi, over its 128Mi request",
		evidenceOf(f, detection.EvidenceOverRequest))
	assert.Empty(t, evidenceOf(f, detection.EvidenceMemoryNodeMax),
		"a node kill gets no limit bounds")
}

func TestFlatRunAndAgainstRequestWording(t *testing.T) {
	assert.Equal(t, "used 100Mi, within its 128Mi request",
		againstRequest(100*mib, 128*mib, true))
	assert.Equal(t, "used 100Mi with no memory request",
		againstRequest(100*mib, 0, true))
	assert.Empty(t, againstRequest(0, 128*mib, false))
	assert.Equal(t, "5h", windowText(5*time.Hour+10*time.Minute))
	assert.Equal(t, "24h", windowText(30*time.Hour))
}
