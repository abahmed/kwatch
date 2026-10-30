package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// boundClaim is a claim bound a month before t0.
func boundClaim(m *inventory.Model) inventory.EntityID {
	claim := newID(kube.KindPVC, "shop", "data")
	put(m, claim, t0.Add(-30*24*time.Hour), map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Bound"),
	})
	return claim
}

func unusedFindings(
	r *detection.Registry, m *inventory.Model, claim inventory.EntityID,
	now time.Time,
) []detection.Finding {
	return r.Evaluate(m, now, claim).Findings
}

func TestClaimUnusedClockStartsWhenTheLastMountGoes(t *testing.T) {
	m := newTestModel()
	claim := boundClaim(m)
	pod := newID(kube.KindPod, "shop", "nightly-1")
	put(m, pod, t0, nil)
	link(m, pod, inventory.Mounts, claim)
	r := detection.NewRegistry(nil, Claim{})
	assert.Empty(t, unusedFindings(r, m, claim, t0))

	gone := t0.Add(time.Hour)
	m.Apply(inventory.Observation{
		Kind: inventory.Gone, Source: "test", At: gone, Entity: pod,
	})
	assert.Empty(t, unusedFindings(r, m, claim, gone),
		"a long-bound claim must not count as unused at once")
	assert.Empty(t, unusedFindings(r, m, claim,
		gone.Add(DefaultUnusedAfter-time.Second)))

	found := unusedFindings(r, m, claim, gone.Add(DefaultUnusedAfter))
	require.Len(t, found, 1)
	assert.Equal(t, "ClaimUnused", found[0].Reason)
	assert.Equal(t, gone, found[0].Since)
}

func TestClaimUnusedClockRestartsWithEachMount(t *testing.T) {
	m := newTestModel()
	claim := boundClaim(m)
	r := detection.NewRegistry(nil, Claim{})
	assert.Empty(t, unusedFindings(r, m, claim, t0))

	pod := newID(kube.KindPod, "shop", "nightly-2")
	put(m, pod, t0.Add(20*time.Hour), nil)
	link(m, pod, inventory.Mounts, claim)
	assert.Empty(t, unusedFindings(r, m, claim, t0.Add(20*time.Hour)))

	gone := t0.Add(21 * time.Hour)
	m.Apply(inventory.Observation{
		Kind: inventory.Gone, Source: "test", At: gone, Entity: pod,
	})
	assert.Empty(t, unusedFindings(r, m, claim, gone))
	assert.Empty(t, unusedFindings(r, m, claim, t0.Add(DefaultUnusedAfter)),
		"the mount in between restarts the day")
}

func TestClaimUnusedWaitsADayAfterARestart(t *testing.T) {
	m := newTestModel()
	claim := boundClaim(m)
	first := t0

	restarted := detection.NewRegistry(nil, Claim{})
	assert.Empty(t, unusedFindings(restarted, m, claim, first))
	assert.Empty(t, unusedFindings(restarted, m, claim,
		first.Add(DefaultUnusedAfter-time.Second)))
	assert.Len(t, unusedFindings(restarted, m, claim,
		first.Add(DefaultUnusedAfter)), 1)
}

func TestClaimUnusedSkipsClaimOfACronJobTemplate(t *testing.T) {
	m := newTestModel()
	claim := boundClaim(m)
	cron := newID(kube.KindCronJob, "shop", "nightly")
	put(m, cron, t0, nil)
	link(m, cron, inventory.References, claim)
	r := detection.NewRegistry(nil, Claim{})

	assert.Empty(t, unusedFindings(r, m, claim, t0))
	assert.Empty(t, unusedFindings(r, m, claim,
		t0.Add(2*DefaultUnusedAfter)))
}
