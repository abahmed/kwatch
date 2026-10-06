package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// The reasoning blames a pod, then the Service in front of it, for the
// very same failures. Neither is news: the fingerprint stays and no
// update follows, however often it flips.
func TestCauseFlipInTheWorkloadChainIsNotAMaterialChange(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 1)
	svc := entity(kube.KindService, "api")
	slice := entity(kube.KindEndpointSlice, "api-abc")
	r.relate(slice, inventory.Backs, svc)
	r.relate(slice, inventory.RoutesTo, pod)
	r.raise(at(0), crashSig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")

	r.m.mu.Lock()
	p := r.m.lookup(deploy)
	podCause := &rootcause.CauseRecord{Rule: "pod-crash", Root: pod}
	svcCause := &rootcause.CauseRecord{Rule: "service-backends", Root: svc,
		RootFindings: []detection.Finding{
			sig(svc, reasons.ServiceNoEndpoints, detection.Warning)}}
	identity := func(c *rootcause.CauseRecord) (string, string) {
		p.Cause = c
		p.causeChain = r.m.inChain(p)
		require.True(t, p.causeChain)
		p.rememberRootReasons()
		return causeIdentity(p), fingerprint(p)
	}
	// Each reason is news once, the first time the reasoning blames it.
	identity(podCause)
	_, withService := identity(svcCause)
	for range 3 {
		_, fromPod := identity(podCause)
		_, fromService := identity(svcCause)
		assert.Equal(t, withService, fromPod)
		assert.Equal(t, withService, fromService)
	}
	r.m.mu.Unlock()
}

// A cause outside the workload chain is still part of the identity.
func TestCauseOutsideTheChainStaysNews(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), crashSig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p := r.m.lookup(deploy)
	before := fingerprint(p)
	p.Cause = &rootcause.CauseRecord{Rule: "node",
		Root: entity(kube.KindNode, "n1")}
	p.causeChain = r.m.inChain(p)
	assert.False(t, p.causeChain)
	assert.NotEqual(t, before, fingerprint(p))
}
