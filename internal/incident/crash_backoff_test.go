package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// backoffRig announces a crash loop of a Deployment that reports all
// its replicas ready, as it does between two crashes, then clears the
// finding the way a quiet back-off does.
func backoffRig(t *testing.T) (*rig, inventory.EntityID) {
	t.Helper()
	r := newRig(t, Config{})
	_, pod := r.workloadRig(t, 1, 1, 6)
	container := entity(kube.KindContainer, "api-1.app")
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	crash := crashSig(pod)
	r.raise(at(0), crash)
	require.Len(t, r.tick(at(DefaultSettle)), 1)
	r.clear(at(5*time.Minute), crash)
	r.tick(at(5 * time.Minute))
	return r, container
}

// A container waiting in back-off is not healthy: the hold is shorter
// than the back-off, so the quiet between two crashes must not read as
// recovery.
func TestCrashBackoffNeverCountsAsHealthy(t *testing.T) {
	r, container := backoffRig(t)
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrStateReason: inventory.Text(reasons.CrashLoopBackOff)})

	for now := 6 * time.Minute; now < 40*time.Minute; now += 4 * time.Minute {
		wantNone(t, r.tick(at(now)))
	}
	assert.Equal(t, Recovering, r.only().State)

	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrStateReason: inventory.Text("")})
	ds := r.tick(at(41 * time.Minute))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}

// A container that crashed within the hold, and runs for the moment,
// is not healthy either.
func TestRecentCrashDoesNotCountAsHealthy(t *testing.T) {
	r, container := backoffRig(t)
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrLastFinished: inventory.Time(at(5*time.Minute + 30*time.Second)),
	})
	wantNone(t, r.tick(at(5*time.Minute+DefaultHold)))
	wantNone(t, r.tick(at(5*time.Minute+30*time.Second+DefaultHold-
		time.Second)))
	ds := r.tick(at(5*time.Minute + 30*time.Second + DefaultHold +
		time.Second))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}
