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

// A notify-tier incident that fails again within RepageWindow of its
// resolve comes back as the same incident, with one "failing again"
// update and no new root message.
func TestNotifyIncidentReopensWithAFailingAgainUpdate(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	first := r.only()
	require.Equal(t, Notify, first.Tier)

	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	wantAction(t, r.tick(at(2*time.Minute+DefaultHold)), Resolve,
		"healthy for 3m0s")

	again := at(20 * time.Minute)
	r.raise(again, web)
	ds := r.tick(again.Add(DefaultReviseSettle))
	wantAction(t, ds, Update, ReasonFailingAgain)
	got := ds[0].Incident
	assert.Equal(t, first.ID, got.ID)
	assert.Equal(t, Notify, got.Tier)
	assert.False(t, got.Delivery.PageHeld(), "a notification is not a held page")
	assert.Contains(t, timelineOf(got), "failing again: 2nd time in 2h")
	assert.Len(t, r.m.Incidents(), 1)

	// And it resolves under the same ID again.
	r.clear(again.Add(3*time.Minute), web)
	r.tick(again.Add(3 * time.Minute))
	ds = r.tick(again.Add(3*time.Minute + 2*ChronicFactor*DefaultHold))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
	assert.Equal(t, first.ID, ds[0].Incident.ID)
}

// digestRecurs raises a digest-tier incident, resolves it, and raises it
// again 20 minutes in. heldFirst keeps the first announcement in the
// digest, as when its digest has not gone out before it resolves.
func digestRecurs(t *testing.T, heldFirst bool) (*rig, Decision) {
	t.Helper()
	r := newRig(t, Config{})
	ds0 := sig(entity(kube.KindPod, "web"), reasons.ServiceUnused,
		detection.Warning)
	r.raise(at(0), ds0)
	ds := r.tick(at(DefaultSettle))
	require.Equal(t, Digest, ds[0].Incident.Tier)
	if heldFirst {
		r.m.HoldAnnouncement(ds[0].Incident.ID)
	}
	r.clear(at(2*time.Minute), ds0)
	r.tick(at(2 * time.Minute))
	r.tick(at(2*time.Minute + DefaultHold))
	if heldFirst {
		r.m.DropAnnouncement(ds[0].Incident.ID)
	}

	r.raise(at(20*time.Minute), ds0)
	ds = r.tick(at(20*time.Minute + DefaultSettle))
	require.Len(t, ds, 1)
	return r, ds[0]
}

// A digest-tier incident that a digest listed comes back as the same
// incident, still at the digest tier, with a "failing again" update that
// the next digest lists as recurring.
func TestDigestIncidentReopensAtDigestTier(t *testing.T) {
	r, d := digestRecurs(t, false)

	assert.Equal(t, Update, d.Action)
	assert.Equal(t, ReasonFailingAgain, d.Reason)
	assert.Equal(t, Digest, d.Incident.Tier, "no interruption")
	assert.Empty(t, d.Incident.Previous, "reopened, not a new incident")
	assert.Len(t, r.m.Incidents(), 1)
	assert.Contains(t, timelineOf(d.Incident), "failing again: 2nd time in 2h")
}

// A digest-tier incident that resolved before any digest listed it was
// never told: its return is a new incident.
func TestUnlistedDigestIncidentIsNotReopened(t *testing.T) {
	_, d := digestRecurs(t, true)

	assert.Equal(t, Announce, d.Action)
	assert.NotEqual(t, ReasonFailingAgain, d.Reason)
}

// After one re-opening the next resolve waits twice as long, so a fast
// flap collapses into fewer messages.
func TestReopenedIncidentWaitsLongerBeforeResolving(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	r.clear(at(2*time.Minute), web)
	r.tick(at(2 * time.Minute))
	wantAction(t, r.tick(at(2*time.Minute+DefaultHold)), Resolve,
		"healthy for 3m0s")

	again := at(20 * time.Minute)
	r.raise(again, web)
	wantAction(t, r.tick(again.Add(DefaultReviseSettle)), Update,
		ReasonFailingAgain)
	r.clear(again.Add(2*time.Minute), web)
	r.tick(again.Add(2 * time.Minute))
	// One recent recovery doubles the hold; the re-opening multiplies it.
	hold := 2 * ChronicFactor * DefaultHold
	wantNone(t, r.tick(again.Add(2*time.Minute+hold-time.Second)))
	wantAction(t, r.tick(again.Add(2*time.Minute+hold)), Resolve,
		"healthy for 24m0s")
}

// The incident's mode follows whichever of its findings the root has at
// the moment; a return that starts with another of the modes the
// incident had is still the same failure.
func TestReopenMatchesAnyModeTheIncidentHad(t *testing.T) {
	for _, again := range []detection.Mode{
		detection.ModeCrashLoop, detection.ModeNotReady,
	} {
		t.Run(string(again), func(t *testing.T) {
			r := newRig(t, Config{})
			crash := podSig("web")
			crash.Mode = detection.ModeCrashLoop
			waiting := sig(crash.Entity, reasons.NotReady, detection.Warning)
			waiting.Mode = detection.ModeNotReady
			r.raise(at(0), crash, waiting)
			wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
			id := r.only().ID
			r.clear(at(2*time.Minute), crash, waiting)
			r.tick(at(2 * time.Minute))
			wantAction(t, r.tick(at(2*time.Minute+DefaultHold)), Resolve,
				"healthy for 3m0s")

			back := crash
			if again == detection.ModeNotReady {
				back = waiting
			}
			r.raise(at(20*time.Minute), back)
			ds := r.tick(at(20*time.Minute + DefaultReviseSettle))
			wantAction(t, ds, Update, ReasonFailingAgain)
			assert.Equal(t, id, ds[0].Incident.ID)
		})
	}
}

// A failure the incident never had is not the same failure.
func TestReopenNeedsAModeTheIncidentHad(t *testing.T) {
	r := newRig(t, Config{})
	crash := podSig("web")
	crash.Mode = detection.ModeCrashLoop
	announced(t, r, crash)
	r.clear(at(2*time.Minute), crash)
	r.tick(at(2 * time.Minute))
	r.tick(at(2*time.Minute + DefaultHold))

	other := sig(crash.Entity, reasons.ImagePullBackOff, detection.Critical)
	other.Mode = detection.ModeImagePull
	r.raise(at(20*time.Minute), other)
	ds := r.tick(at(20*time.Minute + DefaultSettle))
	wantAction(t, ds, Announce, "settled")
}
