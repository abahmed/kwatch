package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The reasoning swinging back to the root it just left, for the same
// failure, does not move the failure again.
func TestManagerDampsCauseSwingingBack(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	first, second := entity(kube.KindConfigMap, "cfg-a"),
		entity(kube.KindConfigMap, "cfg-b")
	r.cause(web.Entity, first, "cfg-a changed")
	r.raise(at(0), web)
	r.tick(at(DefaultSettle))
	r.cause(web.Entity, second, "cfg-b changed")
	r.apply(at(2*time.Minute), detection.Changed, web)

	r.cause(web.Entity, first, "cfg-a changed")
	r.apply(at(2*time.Minute+20*time.Second), detection.Changed, web)

	assert.Equal(t, second, r.ofMember(web).Root,
		"a swing back within the damping keeps the failure")

	r.apply(at(2*time.Minute+flipDamping), detection.Changed, web)
	assert.Equal(t, first, r.ofMember(web).Root,
		"after the damping the revision is real")
}

// A finding that becomes its own root is not "explained by" itself.
func TestManagerReviseToOwnObjectWritesNoSelfNote(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	cfg := entity(kube.KindConfigMap, "cfg")
	r.cause(web.Entity, cfg, "cfg changed")
	r.raise(at(0), web)
	r.tick(at(DefaultSettle))
	delete(r.rule.causes, web.Entity)

	r.apply(at(2*time.Minute), detection.Changed, web)

	for _, p := range r.m.Incidents() {
		for _, e := range p.Timeline {
			self := describe(web.Entity) + " is now explained by " +
				describe(web.Entity)
			assert.False(t, strings.Contains(e.Text, self), e.Text)
		}
	}
}

// ofMember is the incident that holds finding s.
func (r *rig) ofMember(s detection.Finding) Incident {
	r.t.Helper()
	r.m.mu.Lock()
	id := r.m.byMember[s.Key()]
	r.m.mu.Unlock()
	for _, p := range r.m.Incidents() {
		if p.ID == id {
			return p
		}
	}
	r.t.Fatalf("no incident holds %s", s.Entity)
	return Incident{}
}
