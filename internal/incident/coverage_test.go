package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestCoveredWorkloadsNamesWorkloadsOfLiveIncidents(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 3)
	assert.Empty(t, r.m.CoveredWorkloads(at(0)))

	r.raise(at(0), notReadySig(pod))
	assert.True(t, r.m.CoveredWorkloads(at(0))[deploy], "settling covers")
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	assert.True(t, r.m.CoveredWorkloads(at(0))[deploy])

	r.clear(at(2*time.Minute), notReadySig(pod))
	r.tick(at(2 * time.Minute))
	assert.False(t, r.m.CoveredWorkloads(at(0))[deploy],
		"an incident with no failing member covers nothing")
}

func TestReadinessOfReadsReplicasAndFailingPods(t *testing.T) {
	r := newRig(t, Config{})
	deploy, _ := r.workloadRig(t, 3, 1, 4)
	got, ok := ReadinessOf(r.model, deploy)
	require.True(t, ok)
	assert.Equal(t, Readiness{Desired: 3, Ready: 1, Restarts: 4,
		Failing: true}, got)
	assert.True(t, got.Short())
	_, ok = ReadinessOf(r.model, entity(kube.KindPod, "api-1"))
	assert.False(t, ok, "a pod has no replicas")
}

// After a restart, announced incidents that fail again are listed; a
// digest-tier one is not.
func TestRestoredFailingListsAnnouncedIncidentsStillFailing(t *testing.T) {
	src := newRig(t, Config{})
	_, pod := src.workloadRig(t, 2, 1, 3)
	src.raise(at(0), notReadySig(pod))
	other := sig(entity(kube.KindPod, "quiet"), reasons.ServiceUnused,
		detection.Warning)
	src.raise(at(0), other)
	require.Len(t, src.tick(at(DefaultSettle)), 2)
	assert.Empty(t, src.m.RestoredFailing(), "not restored")

	dst := newRig(t, Config{})
	dst.m.Restore(src.m.Export(), time.Time{})
	assert.Empty(t, dst.m.RestoredFailing(), "no member is back yet")
	dst.workloadRig(t, 2, 1, 3)
	dst.raise(at(time.Minute), notReadySig(pod), other)

	got := dst.m.RestoredFailing()
	require.Len(t, got, 1)
	assert.Equal(t, src.of(entity(kube.KindDeployment, "api")).ID, got[0].ID)
}
