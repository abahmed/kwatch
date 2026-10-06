package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const mebi = 1 << 20

// oomContainer is a container that was OOM-killed two minutes before
// t0 and runs again; extra adds usage history or run times.
func oomContainer(extra map[string]inventory.Value) (
	*inventory.Model, inventory.EntityID,
) {
	m := newTestModel()
	id := newID(kube.KindContainer, "shop", "api-1/app")
	attrs := map[string]inventory.Value{
		kube.AttrState:        inventory.Text("running"),
		kube.AttrRestarts:     inventory.Number(5),
		kube.AttrLastReason:   inventory.Text(reasons.OOMKilled),
		kube.AttrLastFinished: inventory.Time(t0.Add(-2 * time.Minute)),
		kube.AttrMemoryLimit:  inventory.Number(512 * mebi),
	}
	for name, value := range extra {
		attrs[name] = value
	}
	put(m, id, t0.Add(-10*time.Minute), attrs)
	return m, id
}

func oomEvidence(t *testing.T, extra map[string]inventory.Value,
) map[string]string {
	t.Helper()
	m, id := oomContainer(extra)
	got := Container{}.Detect(testDetectorContext(m, t0), entityOf(m, id))
	require.Len(t, got, 1)
	require.Equal(t, reasons.OOMKilled, got[0].Reason)
	return evidenceByLabel(got[0].Evidence)
}

func evidenceByLabel(items []detection.Evidence) map[string]string {
	out := map[string]string{}
	for _, item := range items {
		out[item.Label] = item.Value
	}
	return out
}

func TestContainerOOMNamesTheLimitAndThePeakOfTheLastDay(t *testing.T) {
	got := oomEvidence(t, map[string]inventory.Value{
		kube.AttrMemoryPeak24h: inventory.Number(610 * mebi),
	})

	assert.Equal(t, "512Mi", got[detection.EvidenceMemoryLimit])
	assert.Equal(t, "610Mi", got[detection.EvidenceMemoryPeak])
	assert.NotContains(t, got, detection.EvidenceMemoryRise)
}

func TestContainerOOMDescribesASteadyClimbToTheKill(t *testing.T) {
	got := oomEvidence(t, map[string]inventory.Value{
		kube.AttrMemoryPeak24h:      inventory.Number(512 * mebi),
		kube.AttrMemoryPrevStart:    inventory.Number(200 * mebi),
		kube.AttrMemoryPrevPeak:     inventory.Number(512 * mebi),
		kube.AttrMemoryPrevEnded:    inventory.Time(t0.Add(-3 * time.Minute)),
		kube.AttrMemoryPrevSeconds:  inventory.Number(3 * 3600),
		kube.AttrMemoryPrevDrawdown: inventory.Number(2),
	})

	assert.Equal(t, "200Mi to 512Mi over 3h", got[detection.EvidenceMemoryRise])
}

func TestContainerOOMDoesNotCallASawToothClimbSteady(t *testing.T) {
	got := oomEvidence(t, map[string]inventory.Value{
		kube.AttrMemoryPeak24h:      inventory.Number(512 * mebi),
		kube.AttrMemoryPrevStart:    inventory.Number(200 * mebi),
		kube.AttrMemoryPrevPeak:     inventory.Number(512 * mebi),
		kube.AttrMemoryPrevEnded:    inventory.Time(t0.Add(-3 * time.Minute)),
		kube.AttrMemoryPrevSeconds:  inventory.Number(3 * 3600),
		kube.AttrMemoryPrevDrawdown: inventory.Number(40),
	})

	assert.NotContains(t, got, detection.EvidenceMemoryRise)
}

func TestContainerOOMIgnoresARunThatDidNotEndWithTheKill(t *testing.T) {
	got := oomEvidence(t, map[string]inventory.Value{
		kube.AttrMemoryPeak24h:      inventory.Number(512 * mebi),
		kube.AttrMemoryPrevStart:    inventory.Number(200 * mebi),
		kube.AttrMemoryPrevPeak:     inventory.Number(512 * mebi),
		kube.AttrMemoryPrevEnded:    inventory.Time(t0.Add(-5 * time.Hour)),
		kube.AttrMemoryPrevSeconds:  inventory.Number(3 * 3600),
		kube.AttrMemoryPrevDrawdown: inventory.Number(0),
	})

	assert.NotContains(t, got, detection.EvidenceMemoryRise,
		"an older run says nothing about this kill")
}

func TestContainerOOMNotesAKillSoonAfterStarting(t *testing.T) {
	got := oomEvidence(t, map[string]inventory.Value{
		kube.AttrLastStarted: inventory.Time(t0.Add(-2*time.Minute -
			30*time.Second)),
	})

	assert.Equal(t, "30 seconds", got[detection.EvidenceKilledAfter])
}

func TestContainerOOMWithoutUsageHistoryAddsNothingExtra(t *testing.T) {
	got := oomEvidence(t, nil)

	assert.NotContains(t, got, detection.EvidenceMemoryLimit)
	assert.NotContains(t, got, detection.EvidenceMemoryPeak)
	assert.NotContains(t, got, detection.EvidenceMemoryRise)
	assert.NotContains(t, got, detection.EvidenceKilledAfter)
}

func TestQuantityWritesMemoryLikeALimit(t *testing.T) {
	assert.Equal(t, "512Mi", quantity(512*mebi))
	assert.Equal(t, "1.5Gi", quantity(1.5*1024*mebi))
	assert.Equal(t, "2Gi", quantity(2*1024*mebi))
}
