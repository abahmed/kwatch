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

// An incident rooted at a Service that does not exist (a virtual
// "missing Service" a rule inferred from the pod) is saved, and after
// the restart the link is not made again: the pod's failure comes back
// under its Deployment's incident. The restored incident gets no member
// back, yet its pod still fails: it is handed over to the incident that
// holds the pod, never resolved as healthy.
func TestRestoredIncidentOfAMissingRootIsHandedOverByItsMembers(
	t *testing.T,
) {
	svc := entity(kube.KindService, "cert-controller")
	pod := entity(kube.KindPod, "cert-controller-69b-56w4q")
	deploy := entity(kube.KindDeployment, "cert-controller")
	notReady := sig(pod, reasons.ReadinessProbeFailed, detection.Warning)

	old := newRig(t, Config{})
	old.cause(pod, svc, "service cert-controller is missing")
	old.raise(at(0), notReady)
	require.Len(t, tickEvery(old, 0, 5*time.Minute), 1)
	require.Equal(t, svc, old.only().Root)
	records := old.m.Export()
	require.Equal(t, []inventory.EntityID{pod}, records[0].Members)
	oldID := records[0].ID

	grace := 10 * time.Minute
	r := newRig(t, Config{})
	r.relate(pod, inventory.OwnedBy, deploy)
	r.m.Restore(records, at(grace))
	r.raise(at(time.Minute), notReady)
	require.NotEqual(t, oldID, r.idOf(deploy), "the pod is held elsewhere")

	var got []Decision
	for now := time.Minute; now <= grace+10*time.Minute; now += 10 *
		time.Second {
		got = append(got, r.tick(at(now))...)
	}
	for _, d := range got {
		if d.Action != Resolve || d.Incident.ID != oldID {
			continue
		}
		assert.Equal(t, ReasonSuperseded, d.Reason)
		assert.Equal(t, r.idOf(deploy), d.Incident.SupersededBy)
		assert.True(t, d.Handover)
		return
	}
	t.Fatalf("the restored incident never closed: %+v", got)
}

// A record saves at most maxSavedMembers member entities, each once.
func TestMemberEntitiesAreSortedUniqueAndBounded(t *testing.T) {
	p := &Incident{Members: map[detection.Key]detection.Finding{}}
	for i := 0; i < maxSavedMembers+5; i++ {
		s := podSig("web-" + string(rune('a'+i)))
		p.Members[s.Key()] = s
		other := s
		other.Reason = reasons.OOMKilled
		p.Members[other.Key()] = other
	}
	got := memberEntities(p)
	assert.Len(t, got, maxSavedMembers)
	assert.Equal(t, "web-a", got[0].Name)
}

// A second restart inside the restore grace, before any member came
// back, keeps the saved members and reasons: the twice-restored
// incident is still handed over, never resolved as healthy.
func TestRestoredIncidentKeepsItsMembersAcrossASecondRestart(
	t *testing.T,
) {
	svc := entity(kube.KindService, "cert-controller")
	pod := entity(kube.KindPod, "cert-controller-69b-56w4q")
	deploy := entity(kube.KindDeployment, "cert-controller")
	notReady := sig(pod, reasons.ReadinessProbeFailed, detection.Warning)

	old := newRig(t, Config{})
	old.cause(pod, svc, "service cert-controller is missing")
	old.raise(at(0), notReady)
	require.Len(t, tickEvery(old, 0, 5*time.Minute), 1)
	first := old.m.Export()
	oldID := first[0].ID

	grace := 10 * time.Minute
	mid := newRig(t, Config{})
	mid.m.Restore(first, at(grace))
	second := mid.m.Export()
	require.Len(t, second, 1)
	assert.Equal(t, first[0].Members, second[0].Members)
	assert.Equal(t, first[0].Reasons, second[0].Reasons)

	r := newRig(t, Config{})
	r.relate(pod, inventory.OwnedBy, deploy)
	r.m.Restore(second, at(grace))
	r.raise(at(time.Minute), notReady)

	for now := time.Minute; now <= grace+10*time.Minute; now += 10 *
		time.Second {
		for _, d := range r.tick(at(now)) {
			if d.Action != Resolve || d.Incident.ID != oldID {
				continue
			}
			assert.Equal(t, ReasonSuperseded, d.Reason)
			return
		}
	}
	t.Fatal("the twice-restored incident never closed")
}
