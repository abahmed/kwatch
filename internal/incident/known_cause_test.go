package incident

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

// knownCauseRig is a Deployment of two replicas, ready of them ready,
// whose not-ready pod is explained by a node pool. The pool's incident
// was heard on two earlier days, so it is known and demoted.
func knownCauseRig(
	t *testing.T, ready float64,
) (*rig, inventory.EntityID, inventory.EntityID) {
	t.Helper()
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, ready, 0)
	pool := entity(kube.KindNodePool, "workers")
	poolFinding := sig(pool, reasons.NodePSIHigh, detection.Warning)
	poolFinding.Mode = detection.ModeNotReady
	podFinding := notReadySig(pod)
	r.cause(pod, pool, "node pool workers has failing nodes")
	r.raise(at(0), poolFinding, podFinding)
	r.m.mu.Lock()
	p := r.m.lookup(pool)
	require.NotNil(t, p)
	for _, ago := range []time.Duration{48 * time.Hour, 30 * time.Hour} {
		p.History = append(p.History, Occurrence{
			Mode: p.Mode, Opened: at(-ago), Resolved: at(-ago + time.Hour),
			Heard: true})
	}
	r.m.mu.Unlock()
	r.apply(at(time.Second), detection.Changed, poolFinding, podFinding)
	require.Equal(t, Digest, r.of(pool).Tier, "known: demoted")
	return r, deploy, pool
}

// A workload with nothing ready past the boot window is not boot noise
// when its failure is explained by another root: the known incident of
// that root must speak, as one rooted at the workload would.
func TestKnownCauseWithNothingReadyPastBootIsNotDemoted(t *testing.T) {
	r, _, pool := knownCauseRig(t, 0)
	ds := r.tick(at(DefaultSettle))
	require.Equal(t, Digest, ds[0].Incident.Tier)

	wantNone(t, r.tick(at(kube.BootWindow-time.Minute)))
	ds = r.tick(at(kube.BootWindow))

	require.Len(t, ds, 1)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
	assert.Equal(t, pool, ds[0].Incident.Root)
}

// The same known incident stays demoted while the workload it holds
// still has a ready replica: one unready pod is not an outage.
func TestKnownCauseWithAReadyReplicaStaysDemoted(t *testing.T) {
	r, _, _ := knownCauseRig(t, 1)
	ds := r.tick(at(DefaultSettle))
	require.Equal(t, Digest, ds[0].Incident.Tier)

	wantNone(t, r.tick(at(kube.BootWindow)))
	wantNone(t, r.tick(at(time.Hour)))
}
