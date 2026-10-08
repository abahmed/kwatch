package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// storyRig puts a Deployment with a crash-looping pod and the Service
// in front of it into the model.
func storyRig(t *testing.T, r *rig) (
	deploy, svc inventory.EntityID, crash detection.Finding,
) {
	t.Helper()
	deploy, pod := r.workloadRig(t, 2, 1, 1)
	svc = entity(kube.KindService, "api")
	slice := entity(kube.KindEndpointSlice, "api-abc")
	r.relate(slice, inventory.Backs, svc)
	r.relate(slice, inventory.RoutesTo, pod)
	return deploy, svc, crashSig(pod)
}

// The reasoning moves an announced crash loop to the Service in front
// of it, and the failure then comes back to the Deployment. It stays
// with the moved incident, also after a restart: staging restarts every
// night, and the churn began right after one.
func TestFailureBackToFormerRootStaysAfterRestart(t *testing.T) {
	src := newRig(t, Config{})
	_, svc, crash := storyRig(t, src)
	src.raise(at(0), crash)
	wantAction(t, src.tick(at(DefaultSettle)), Announce, "settled")
	id := src.only().ID
	src.m.mu.Lock()
	src.m.reroot(at(2*time.Minute), src.m.incidents[id], svc)
	src.m.mu.Unlock()

	dst := newRig(t, Config{})
	_, _, crash = storyRig(t, dst)
	dst.m.Restore(src.m.Export(), at(2*time.Minute))
	dst.raise(at(3*time.Minute), crash)
	dst.tick(at(3*time.Minute + DefaultSettle))

	require.Len(t, dst.m.Export(), 1, "no new incident for the old root")
	assert.Equal(t, svc, dst.only().Root)
	assert.Len(t, dst.only().Members, 1, "the failure joined it")
}
