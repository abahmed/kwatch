package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reason that matches a pager route may appear only after the
// announcement. The announced route must widen with every update that
// is sent, and survive a restart whole, so the resolve still finds it.
func TestAnnouncedRouteWidensWithEachSentUpdate(t *testing.T) {
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	first := r.only().AnnouncedRoute
	require.NotNil(t, first)
	assert.NotContains(t, first.Reasons, crashSig(pod).Reason)

	r.raise(at(2*time.Minute), crashSig(pod))
	wantAction(t, r.tick(at(2*time.Minute)), Update, ReasonMaterialChange)
	widened := r.only().AnnouncedRoute
	assert.Contains(t, widened.Reasons, notReadySig(pod).Reason)
	assert.Contains(t, widened.Reasons, crashSig(pod).Reason)

	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), time.Time{})
	assert.Equal(t, widened, fresh.only().AnnouncedRoute)
}

func TestWidenRouteKeepsTheHighestSeverity(t *testing.T) {
	told := &AnnouncedRoute{
		Namespaces: []string{"b"}, Reasons: []string{"X"},
		Severity: "critical",
	}
	told.widen(&AnnouncedRoute{
		Namespaces: []string{"a"}, Reasons: []string{"Y"},
		Severity: "warning",
	})
	assert.Equal(t, []string{"a", "b"}, told.Namespaces)
	assert.Equal(t, []string{"X", "Y"}, told.Reasons)
	assert.Equal(t, "critical", told.Severity)

	told.widen(&AnnouncedRoute{Severity: "critical"})
	low := &AnnouncedRoute{Severity: "info"}
	low.widen(&AnnouncedRoute{Severity: "warning"})
	assert.Equal(t, "warning", low.Severity)
}
