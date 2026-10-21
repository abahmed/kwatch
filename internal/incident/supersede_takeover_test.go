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

// fourAnnouncedThenSharedCause announces four crash-looping workloads,
// then, two minutes later, makes a DaemonSet their cause.
func fourAnnouncedThenSharedCause(t *testing.T) (*rig, time.Time) {
	t.Helper()
	r := newRig(t, Config{})
	var sigs []detection.Finding
	for _, name := range []string{"cart", "search", "billing", "media"} {
		pod, _ := workload(r, name)
		sigs = append(sigs,
			sig(pod, reasons.CrashLoopBackOff, detection.Warning))
	}
	r.raise(at(0), sigs...)
	require.Len(t, r.tick(at(DefaultSettle)), 4, "setup: four announcements")
	cni := entity(kube.KindDaemonSet, "cni")
	for _, s := range sigs {
		c := r.cause(s.Entity, cni, "cni explains")
		c.Began = at(30 * time.Second)
		r.rule.causes[s.Entity] = c
	}
	now := at(2 * time.Minute)
	r.raise(now, sig(cni, reasons.CrashLoopBackOff, detection.Warning))
	r.apply(now, detection.Changed, sigs...)
	return r, now
}

// Incidents that lost their failures to the one that took them over close
// with its update: one of them became the incident of the new cause and
// says so after ReviseSettle, so the thread that says "Moved" must not
// come half a minute before the message it points to.
func TestMovedClosesComeWithTheUpdateOfTheIncidentThatTookOver(t *testing.T) {
	r, now := fourAnnouncedThenSharedCause(t)

	early, wake := r.m.Tick(now)
	wantNone(t, early)
	assert.Equal(t, DefaultReviseSettle, wake,
		"the closes wait for the revision's own deadline")
	wantNone(t, r.tick(now.Add(time.Second)))

	ds := r.tick(now.Add(DefaultReviseSettle))
	require.Len(t, ds, 4, "the update and the three closes, together")
	assert.Equal(t, Update, ds[0].Action)
	assert.Equal(t, ReasonCauseRevised, ds[0].Reason)
	for _, d := range ds[1:] {
		assert.Equal(t, Resolve, d.Action)
		assert.Equal(t, ReasonSuperseded, d.Reason)
		assert.Equal(t, ds[0].Incident.ID, d.Incident.SupersededBy)
	}
}
