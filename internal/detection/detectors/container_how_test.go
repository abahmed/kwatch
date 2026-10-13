package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// jumpAttrs are the readings of a run that ended in the kill: 180Mi,
// then 512Mi gap seconds later.
func jumpAttrs(gap float64) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrMemoryPeak24h:      inventory.Number(512 * mebi),
		kube.AttrMemoryPrevStart:    inventory.Number(170 * mebi),
		kube.AttrMemoryPrevPeak:     inventory.Number(512 * mebi),
		kube.AttrMemoryPrevEnded:    inventory.Time(t0.Add(-2 * time.Minute)),
		kube.AttrMemoryPrevSeconds:  inventory.Number(600),
		kube.AttrMemoryPrevDrawdown: inventory.Number(0),
		kube.AttrMemoryPrevBefore:   inventory.Number(180 * mebi),
		kube.AttrMemoryPrevLast:     inventory.Number(512 * mebi),
		kube.AttrMemoryPrevGapSecs:  inventory.Number(gap),
	}
}

func TestContainerOOMNotesAJumpInTheLastReadings(t *testing.T) {
	got := oomEvidence(t, jumpAttrs(40))

	assert.Equal(t, "180Mi to 512Mi within 40s",
		got[detection.EvidenceMemoryJump])
}

func TestContainerOOMIgnoresASlowRiseBetweenReadings(t *testing.T) {
	got := oomEvidence(t, jumpAttrs(600))

	assert.NotContains(t, got, detection.EvidenceMemoryJump,
		"ten minutes between readings is not sudden")
}

// crashingPod is a container that waits in back-off in a pod made an
// hour ago.
func crashingPod(restarts float64, ran time.Duration) (
	*inventory.Model, inventory.EntityID,
) {
	m := newTestModel()
	pod := newID(kube.KindPod, "shop", "api-1")
	id := newID(kube.KindContainer, "shop", "api-1/app")
	put(m, pod, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(t0.Add(-time.Hour)),
	})
	put(m, id, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrState:        inventory.Text("waiting"),
		kube.AttrStateReason:  inventory.Text("CrashLoopBackOff"),
		kube.AttrRestarts:     inventory.Number(restarts),
		kube.AttrLastStarted:  inventory.Time(t0.Add(-time.Minute - ran)),
		kube.AttrLastFinished: inventory.Time(t0.Add(-time.Minute)),
	})
	link(m, id, inventory.PartOf, pod)
	return m, id
}

func TestCrashLoopSaysHowLongTheLastRunLasted(t *testing.T) {
	m, id := crashingPod(14, 4*time.Second)

	got := Container{}.Detect(testDetectorContext(m, t0), entityOf(m, id))

	require.Len(t, got, 1)
	have := evidenceByLabel(got[0].Evidence)
	assert.Equal(t, "4s", have[detection.EvidenceRanFor])
	assert.Equal(t, "1h", have[detection.EvidenceRestartSpan])
}

func TestOneRestartIsNotARhythm(t *testing.T) {
	m, id := crashingPod(1, 4*time.Second)

	got := Container{}.Detect(testDetectorContext(m, t0), entityOf(m, id))

	require.Len(t, got, 1)
	have := evidenceByLabel(got[0].Evidence)
	assert.Equal(t, "4s", have[detection.EvidenceRanFor])
	assert.NotContains(t, have, detection.EvidenceRestartSpan)
}
