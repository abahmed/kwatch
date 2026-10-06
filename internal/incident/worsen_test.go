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

// A Deployment incident announced while its pod was only not ready says
// so once when the pod begins to crash-loop, and not again for more
// crashing pods or the same loop.
func TestCrashLoopAfterAnnounceIsOneUpdate(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	require.Equal(t, deploy, r.only().Root)

	r.raise(at(2*time.Minute), crashSig(pod))
	ds := r.tick(at(2 * time.Minute))
	wantAction(t, ds, Update, ReasonMaterialChange)

	other := entity(kube.KindPod, "api-2")
	r.relate(other, "owned-by", deploy)
	r.raise(at(15*time.Minute), crashSig(other))
	wantNone(t, r.tick(at(15*time.Minute)))
	wantNone(t, r.tick(at(40*time.Minute)))
}

// An incident that crash-looped from the start has nothing to add.
func TestCrashLoopFromTheStartIsNotAnUpdate(t *testing.T) {
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod), crashSig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	wantNone(t, r.tick(at(time.Hour)))
}

// Two material changes less than MaterialGap apart are one message
// later, unless the tier rises.
func TestMaterialChangesAreSpacedByTheGap(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.raise(at(2*time.Minute), crashSig(pod))
	wantAction(t, r.tick(at(2*time.Minute)), Update, ReasonMaterialChange)

	own := sig(deploy, reasons.DeploymentUnavailable, detection.Warning)
	r.raise(at(4*time.Minute), own)
	wantNone(t, r.tick(at(4*time.Minute)))
	_, next := r.m.Tick(at(5 * time.Minute))
	assert.Equal(t, 7*time.Minute, next, "wakes when the gap ends")
	wantNone(t, r.tick(at(2*time.Minute+MaterialGap-time.Second)))
	wantAction(t, r.tick(at(2*time.Minute+MaterialGap)), Update,
		ReasonMaterialChange)
}

// A known incident is demoted to the digest, until it crash-loops
// past the boot window; boot noise inside the window still demotes.
func TestKnownIncidentStaysDemotedDuringBoot(t *testing.T) {
	r, _ := knownRig(t, crashSig)
	ds := r.tick(at(DefaultSettle))
	require.Len(t, ds, 1)
	assert.Equal(t, Digest, ds[0].Incident.Tier)
	wantNone(t, r.tick(at(kube.BootWindow-time.Minute)))
}

func TestKnownIncidentCrashLoopPastBootIsNotDemoted(t *testing.T) {
	r, _ := knownRig(t, crashSig)
	ds := r.tick(at(DefaultSettle))
	require.Equal(t, Digest, ds[0].Incident.Tier)
	_, next := r.m.Tick(at(time.Minute))
	assert.Equal(t, kube.BootWindow-time.Minute, next,
		"wakes at the end of the boot window")

	ds = r.tick(at(kube.BootWindow))
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
	wantNone(t, r.tick(at(kube.BootWindow+time.Hour)))
}

// A known workload with nothing ready past the boot window is not boot
// noise either, even when no container is Critical.
func TestKnownIncidentWithNothingReadyPastBootIsNotDemoted(t *testing.T) {
	r, _ := knownRig(t, notReadySig)
	ds := r.tick(at(DefaultSettle))
	require.Equal(t, Digest, ds[0].Incident.Tier)
	wantNone(t, r.tick(at(kube.BootWindow-time.Minute)))
	ds = r.tick(at(kube.BootWindow))
	require.Len(t, ds, 1)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
}

// knownRig raises the finding of a Deployment with no ready replica whose
// problem the history says people heard about for more than a day.
func knownRig(
	t *testing.T, finding func(inventory.EntityID) detection.Finding,
) (*rig, inventory.EntityID) {
	t.Helper()
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 0, 3)
	f := finding(pod)
	r.raise(at(0), f)
	r.m.mu.Lock()
	p := r.m.lookup(deploy)
	for _, ago := range []time.Duration{48 * time.Hour, 30 * time.Hour} {
		p.History = append(p.History, Occurrence{
			Mode: f.Mode, Opened: at(-ago), Resolved: at(-ago + time.Hour),
			Heard: true})
	}
	r.m.mu.Unlock()
	r.apply(at(time.Second), detection.Changed, f)
	require.Equal(t, Digest, r.only().Tier, "known: demoted")
	return r, pod
}

// An autoscaler at its maximum for 30 minutes goes from the digest to
// a notification, once.
func TestHPAMaxedOutLongEscalatesOnce(t *testing.T) {
	r := newRig(t, Config{})
	hpa := sig(entity(kube.KindHPA, "api"), reasons.HPAMaxedOut,
		detection.Warning)
	r.raise(at(0), hpa)
	ds := r.tick(at(DefaultSettle))
	require.Equal(t, Digest, ds[0].Incident.Tier)
	wantNone(t, r.tick(at(HPAStuckAfter-time.Second)))

	ds = r.tick(at(HPAStuckAfter))
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
	wantNone(t, r.tick(at(HPAStuckAfter+time.Hour)))
}
