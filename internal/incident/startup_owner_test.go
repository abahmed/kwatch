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

// At startup a pod can be listed before the owners above its
// ReplicaSet: its failure first opens under the ReplicaSet, then moves
// to the Deployment once that is listed. The ReplicaSet incident nobody
// heard of is dropped, not kept as a second problem.
func TestManagerDropsUntoldIncidentWhoseOwnerArrivedLate(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	rs := entity(kube.KindReplicaSet, "web-5d4f")
	deploy := entity(kube.KindDeployment, "web")
	r.observe(web.Entity)
	r.relate(web.Entity, inventory.OwnedBy, rs)
	r.raise(at(0), web)
	require.Equal(t, rs, r.ofMember(web).Root)

	r.observe(rs)
	r.relate(rs, inventory.OwnedBy, deploy)
	r.observe(deploy)
	r.apply(at(3*time.Second), detection.Changed, web)
	r.tick(at(4 * time.Second))

	roots := map[inventory.EntityID]bool{}
	for _, p := range r.m.Incidents() {
		roots[p.Root] = true
	}
	assert.Equal(t, map[inventory.EntityID]bool{deploy: true}, roots)
}

// An incident that recovered before it was told stays, marked untold.
func TestManagerKeepsUntoldIncidentThatRecovered(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	r.raise(at(0), web)
	r.clear(at(10*time.Second), web)
	r.tick(at(20 * time.Second))

	got := r.of(web.Entity)
	assert.Equal(t, Resolved, got.State)
	assert.True(t, got.Untold())
	assert.NotEmpty(t, WhyOf(got, at(time.Hour)).AlertLevel,
		"it still says why it was at its level")
}

// An untold incident at a ReplicaSet whose other failure recovered
// while it was a member is dropped all the same once the ReplicaSet's
// owner is listed: a ReplicaSet's failures are its Deployment's, never
// a problem of their own beside it.
func TestManagerDropsUntoldChildIncidentWhenOnlySomeFailuresMoved(
	t *testing.T,
) {
	r := newRig(t, Config{})
	web, api := podSig("web"), podSig("api")
	rs := entity(kube.KindReplicaSet, "web-5d4f")
	deploy := entity(kube.KindDeployment, "web")
	for _, s := range []detection.Finding{web, api} {
		r.observe(s.Entity)
		r.relate(s.Entity, inventory.OwnedBy, rs)
		r.raise(at(0), s)
	}
	require.Equal(t, rs, r.ofMember(web).Root)
	require.Equal(t, rs, r.ofMember(api).Root)
	r.clear(at(2*time.Second), api)

	r.observe(rs)
	r.relate(rs, inventory.OwnedBy, deploy)
	r.observe(deploy)
	r.apply(at(3*time.Second), detection.Changed, web)
	r.tick(at(4 * time.Second))

	for _, p := range r.m.Incidents() {
		assert.NotEqual(t, rs, p.Root, "a standalone ReplicaSet incident")
	}
	assert.Equal(t, deploy, r.ofMember(web).Root)
}

// A failure that recovered while it was still rooted at a ReplicaSet
// listed before its Deployment is not kept as a problem of its own: the
// ReplicaSet has an owner by now, and its failures are the owner's.
func TestManagerDropsUntoldChildIncidentThatRecovered(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	rs := entity(kube.KindReplicaSet, "web-5d4f")
	deploy := entity(kube.KindDeployment, "web")
	r.observe(web.Entity)
	r.relate(web.Entity, inventory.OwnedBy, rs)
	r.raise(at(0), web)
	require.Equal(t, rs, r.ofMember(web).Root)

	r.observe(rs)
	r.relate(rs, inventory.OwnedBy, deploy)
	r.observe(deploy)
	r.clear(at(3*time.Second), web)
	r.tick(at(20 * time.Second))

	for _, p := range r.m.Incidents() {
		assert.NotEqual(t, rs, p.Root, "a standalone ReplicaSet incident")
	}
}
