package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// startFlapping puts the only incident of r in the Flapping state, as
// three recoveries inside the flap window would.
func startFlapping(r *rig) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	for _, p := range r.m.incidents {
		p.State = Flapping
	}
}

func TestFlappingIncidentSaysSoWhenTheFailureGrows(t *testing.T) {
	r := newRig(t, Config{})
	deploy := entity(kube.KindDeployment, "web")
	first := podOf(r, deploy, "web-a")
	r.raise(at(0), first)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	startFlapping(r)

	one := []detection.Finding{podOf(r, deploy, "web-b")}
	r.raise(at(time.Minute), one...)
	wantNone(t, r.tick(at(time.Minute)))

	var more []detection.Finding
	for _, name := range []string{"web-c", "web-d", "web-e", "web-f"} {
		more = append(more, podOf(r, deploy, name))
	}
	r.raise(at(2*time.Minute), more...)
	ds := r.tick(at(2 * time.Minute))
	wantAction(t, ds, Update, ReasonMaterialChange)
	assert.Equal(t, Flapping, ds[0].Incident.State)
	wantNone(t, r.tick(at(3*time.Minute)))
}

func TestFlappingPageKeepsItsReminder(t *testing.T) {
	r, start := pagedRig(t)
	startFlapping(r)

	wantNone(t, r.tick(start.Add(PageRemindAfter-time.Second)))
	wantAction(t, r.tick(start.Add(PageRemindAfter)), Update, ReasonReminder)
}
