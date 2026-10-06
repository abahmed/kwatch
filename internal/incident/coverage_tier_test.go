package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// quietRig announces an incident on a Deployment with nothing ready and
// then makes it digest-tier, as known boot noise would be.
func quietRig(t *testing.T) (*rig, inventory.EntityID) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 0, 3)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	r.m.mu.Lock()
	for _, p := range r.m.incidents {
		p.Tier = Digest
	}
	r.m.mu.Unlock()
	return r, deploy
}

// A digest-tier incident is quiet on purpose, and inside the boot window
// that is right: it still speaks for its workload, so the coverage
// backstop does not call it lost.
func TestCoveredWorkloadsCountsDigestIncidentInsideBootWindow(t *testing.T) {
	r, deploy := quietRig(t)
	assert.True(t, r.m.CoveredWorkloads(at(time.Minute))[deploy])
	assert.True(t,
		r.m.CoveredWorkloads(at(kube.BootWindow - time.Second))[deploy])
}

// Past the boot window a workload with nothing ready is not boot noise.
// A digest incident that tier would raise is not covered until escalate
// has raised it; once raised, it covers again.
func TestCoveredWorkloadsDropsDigestIncidentOwedEscalation(t *testing.T) {
	r, deploy := quietRig(t)
	late := at(kube.BootWindow + time.Minute)
	assert.False(t, r.m.CoveredWorkloads(late)[deploy],
		"it should already speak")

	r.tick(late)
	assert.Equal(t, Notify, r.only().Tier, "escalate raised it")
	assert.True(t, r.m.CoveredWorkloads(late)[deploy])
}

// A digest incident that stays digest even when persistent (here only
// informational findings) is quiet by design, however long it lasts,
// so it keeps covering and the backstop never hands it back.
func TestCoveredWorkloadsKeepsDigestIncidentTierWillNotRaise(t *testing.T) {
	r, deploy := quietRig(t)
	r.m.mu.Lock()
	for _, p := range r.m.incidents {
		for key, s := range p.Members {
			s.Severity = detection.Info
			p.Members[key] = s
		}
	}
	r.m.mu.Unlock()
	assert.True(t, r.m.CoveredWorkloads(at(time.Hour))[deploy])
}

// A digest item does not speak for the workloads in its impact.
func TestCoveredWorkloadsQuietIncidentSkipsItsImpact(t *testing.T) {
	r, _ := quietRig(t)
	other := entity(kube.KindDeployment, "other")
	r.m.mu.Lock()
	for _, p := range r.m.incidents {
		p.Impact = append(p.Impact, other)
	}
	r.m.mu.Unlock()
	assert.False(t, r.m.CoveredWorkloads(at(time.Minute))[other])
}

// A routine incident (same time of day on three days) that outlasts the
// boot window with a crash loop is not routine boot noise.
func TestTierRoutineIncidentThatPersistsIsNotDigest(t *testing.T) {
	deploy := entity(kube.KindDeployment, "web")
	p := incidentOf(deploy, nil,
		sig(deploy, reasons.CrashLoopBackOff, detection.Critical))
	p.Occurrences = dailyOccurrences(3, 3, 3)
	assert.Equal(t, Digest, tier(p))
	p.persistent = true
	assert.Equal(t, Notify, tier(p))
}

// Handing back findings that already belong to an incident, as the
// coverage check does, changes nothing: no second incident, no extra
// member, no message.
func TestApplyIsIdempotentWhenFindingsAlreadyHaveAnIncident(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 0, 3)
	r.raise(at(0), notReadySig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	before := r.m.Export()
	r.m.TakeOpened()

	r.m.Apply(r.snapshot(at(time.Minute)),
		[]inventory.EntityID{deploy, pod}, nil)

	assert.Equal(t, before, r.m.Export())
	assert.Empty(t, r.m.TakeOpened(), "no new incident")
	wantNone(t, r.tick(at(2*time.Minute)))
}
