package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// chainRig announces an incident of two pods of deployment web, which a
// Service backs through an EndpointSlice.
func chainRig(t *testing.T) (
	*rig, inventory.EntityID, inventory.EntityID, [2]detection.Finding,
) {
	r := newRig(t, Config{})
	deploy := entity(kube.KindDeployment, "web")
	svc := entity(kube.KindService, "web")
	slice := entity(kube.KindEndpointSlice, "web-x")
	a, b := podOf(r, deploy, "web-a"), podOf(r, deploy, "web-b")
	r.relate(slice, inventory.Backs, svc)
	r.relate(slice, inventory.RoutesTo, a.Entity, b.Entity)
	r.raise(at(0), a, b)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	return r, deploy, svc, [2]detection.Finding{a, b}
}

// Blaming the same failures on the workload's Service is no revision:
// the root stays as announced and nothing is sent.
func TestSameChainFlipKeepsRootAndStaysQuiet(t *testing.T) {
	r, deploy, svc, pods := chainRig(t)
	id := r.idOf(deploy)

	for _, pod := range pods {
		r.cause(pod.Entity, svc, "service web has no endpoints")
		r.apply(at(2*time.Minute), detection.Changed, pod)
	}

	wantNone(t, r.tick(at(2*time.Minute+time.Second)))
	wantNone(t, r.tick(at(2*time.Minute+DefaultReviseSettle)))
	got := r.of(deploy)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, deploy, got.Root)
	assert.Len(t, got.Members, 2)
}

// A cause outside the workload's chain is still a revision.
func TestOtherChainStillRevises(t *testing.T) {
	r, deploy, _, pods := chainRig(t)
	node := entity(kube.KindNode, "n1")

	for _, pod := range pods {
		r.cause(pod.Entity, node, "node n1 is out of memory")
		r.apply(at(2*time.Minute), detection.Changed, pod)
	}

	ds := r.tick(at(2*time.Minute + DefaultReviseSettle))
	wantAction(t, ds, Update, ReasonCauseRevised)
	assert.Equal(t, node, ds[0].Incident.Root)
	_ = deploy
}

// A superseded incident whose failures are exactly those of the new
// one, which opened within ReviseSettle, closes without a message; a
// page keeps its resolve because that closes its alert.
func TestQuietSupersedeNeedsSameFailuresAndAFreshTarget(t *testing.T) {
	m := NewManager(Config{}, nil)
	key := detection.Key{Entity: entity(kube.KindPod, "a"), Reason: "R"}
	told := map[detection.Key]struct{}{key: {}}
	target := &Incident{ID: "new", Opened: at(time.Minute),
		Members: map[detection.Key]detection.Finding{key: {}}}
	m.incidents["new"] = target
	old := &Incident{SupersededBy: "new", decided: told, Tier: Notify}
	now := at(time.Minute + 10*time.Second)

	assert.True(t, m.quietSupersede(old, now))
	assert.False(t, m.quietSupersede(old, at(5*time.Minute)),
		"a target older than ReviseSettle is a real handover")
	old.Tier = Page
	assert.False(t, m.quietSupersede(old, now))
	old.Tier = Notify
	old.Delivery.HoldAtNotify()
	assert.False(t, m.quietSupersede(old, now), "a held page paged")
	old.Delivery = Delivery{}
	old.Delivery.MarkPaged()
	assert.False(t, m.quietSupersede(old, now), "a paged alert is open")
	old.Delivery.ClosePage()
	target.Members[detection.Key{Entity: entity(kube.KindPod, "b")}] =
		detection.Finding{}
	assert.False(t, m.quietSupersede(old, now), "new failures are news")
}

// A finding whose evidence aged out would move to a root with no cause;
// while the announced root still fails it stays where it is.
func TestExplanationLapsedKeepsAFindingWhileTheRootFails(t *testing.T) {
	m := NewManager(Config{}, nil)
	root := entity(kube.KindNode, "guard")
	own := detection.Key{Entity: root, Reason: "R"}
	p := &Incident{ID: "old", Root: root, State: Open,
		Members: map[detection.Key]detection.Finding{
			own: {Entity: root, Reason: "R"}}}
	m.incidents["old"] = p

	assert.True(t, m.explanationLapsed("old", placement{}))
	assert.False(t, m.explanationLapsed("old", placement{
		cause: &rootcause.CauseRecord{}}), "a real new cause is a revision")
	delete(p.Members, own)
	assert.False(t, m.explanationLapsed("old", placement{}),
		"nothing left that still fails")
	p.Members[own] = detection.Finding{Entity: root, Reason: "R",
		Symptom: true}
	assert.False(t, m.explanationLapsed("old", placement{}),
		"a symptom is not the root failing")
}

// A quiet supersede sends no decision, but it is still a resolve: the
// startup summary and roll-ups it was listed in must be checked for
// closing, so the manager reports it once.
func TestQuietSupersedeAsksForAListingCheck(t *testing.T) {
	m := NewManager(Config{}, nil)
	key := detection.Key{Entity: entity(kube.KindPod, "a"), Reason: "R"}
	m.incidents["new"] = &Incident{ID: "new", Opened: at(time.Minute),
		Members: map[detection.Key]detection.Finding{key: {}}}
	m.incidents["old"] = &Incident{ID: "old", SupersededBy: "new",
		State: Open, Tier: Notify, Announced: at(0),
		decided: map[detection.Key]struct{}{key: {}}}
	assert.Empty(t, m.TakeQuietResolves())

	ds, _ := m.Tick(at(time.Minute + 10*time.Second))

	assert.Empty(t, ds, "no message")
	assert.Equal(t, Resolved, m.incidents["old"].State)
	assert.Equal(t, []string{"old"}, m.TakeQuietResolves(),
		"but a listing may be over")
	assert.Empty(t, m.TakeQuietResolves(), "reported once")
	assert.True(t, m.ResolvedQuietly("old"))
	assert.False(t, m.ResolvedQuietly("new"))
}
