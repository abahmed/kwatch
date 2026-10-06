package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// proximityRig builds shop/api (Deployment > ReplicaSet > pod on node n1)
// that uses ConfigMap app-config and is fronted by Service api, next to
// an unrelated ConfigMap and a Service of another workload.
type proximityRig struct {
	*rig
	root, config, service, node, other inventory.EntityID
}

func newProximityRig(t *testing.T) proximityRig {
	r := newRig(t, Config{})
	p := proximityRig{rig: r,
		root:    entity(kube.KindDeployment, "api"),
		config:  entity(kube.KindConfigMap, "app-config"),
		service: entity(kube.KindService, "api"),
		node:    inventory.CoreID(kube.KindNode, "", "n1"),
		other:   entity(kube.KindConfigMap, "unrelated")}
	rs := entity(kube.KindReplicaSet, "api-1")
	pod := entity(kube.KindPod, "api-1-a")
	for _, id := range []inventory.EntityID{p.root, p.config, p.service,
		p.node, p.other, rs, pod} {
		r.observe(id)
	}
	r.relate(rs, inventory.OwnedBy, p.root)
	r.relate(pod, inventory.OwnedBy, rs)
	r.relate(pod, inventory.RunsOn, p.node)
	r.relate(p.root, inventory.References, p.config)
	r.relate(p.service, inventory.Selects, pod)
	return p
}

func edit(path, before, after string) inventory.Change {
	return inventory.Change{Fields: []inventory.FieldChange{{
		Path: path, Before: before, After: after}}}
}

func envEdit(actor string) inventory.Change {
	return inventory.Change{Actor: actor, Fields: []inventory.FieldChange{{
		Path: "containers[app].env.DB_HOST", Before: "a", After: "b"}}}
}

// Changes are ordered by graph distance from the root: the object
// itself, what it uses, its Service, its node, then the namespace.
func TestRecentChangesRanksByGraphProximity(t *testing.T) {
	p := newProximityRig(t)
	// Newest first would be the reverse of the expected order.
	p.change(p.root, 1*time.Minute, envEdit("alice"))
	p.change(p.config, 2*time.Minute, dataEdit("bob"))
	p.change(p.service, 3*time.Minute, edit("spec.selector", "a=1", "a=2"))
	p.change(p.node, 4*time.Minute, edit("labels.pool", "a", "b"))
	p.change(p.other, 5*time.Minute, dataEdit("eve"))

	got := RecentChanges(p.model, p.root, at(10*time.Minute))

	// Only three are named; the node and the namespace are cut.
	assert.Equal(t, []inventory.EntityID{p.root, p.config, p.service},
		entitiesOf(got))

	near := NearChanges(p.model, p.root, at(10*time.Minute), time.Time{}, nil)
	assert.Equal(t, []inventory.EntityID{p.root, p.config, p.service},
		entitiesOf(near), "a namespace-level change is not near")
}

func TestNearChangesSkipsTheBlamedChangeAndLateChanges(t *testing.T) {
	p := newProximityRig(t)
	p.change(p.root, 1*time.Minute, envEdit("alice"))
	p.change(p.config, 2*time.Minute, dataEdit("bob"))
	p.change(p.service, 9*time.Minute, edit("spec.ports", "80", "81"))
	blamed := p.model.Changes(p.root, time.Time{})[0]

	got := NearChanges(p.model, p.root, at(10*time.Minute),
		at(5*time.Minute), &blamed)

	assert.Equal(t, []inventory.EntityID{p.config}, entitiesOf(got),
		"the blamed change and one made after the failure began drop out")
}

func TestNodeCreationIsNotAChangeWorthNaming(t *testing.T) {
	p := newProximityRig(t)
	p.change(p.node, time.Minute, inventory.Change{Created: true})

	assert.Empty(t, RecentChanges(p.model, p.root, at(10*time.Minute)))
}

func TestIncidentOnsetIsTheEarliestRealFindingStart(t *testing.T) {
	member := func(since time.Duration, advisory bool) detection.Finding {
		return detection.Finding{Since: at(since), Advisory: advisory}
	}
	p := Incident{Opened: at(9 * time.Minute),
		Members: map[detection.Key]detection.Finding{
			{Reason: "a"}: member(5*time.Minute, false),
			{Reason: "b"}: member(time.Minute, true),
			{Reason: "c"}: member(7*time.Minute, false)}}

	assert.Equal(t, at(5*time.Minute), p.Onset())
	assert.Equal(t, at(9*time.Minute), Incident{Opened: at(9 * time.
		Minute)}.Onset())
}

func entitiesOf(changes []inventory.Change) []inventory.EntityID {
	var out []inventory.EntityID
	for _, c := range changes {
		out = append(out, c.Entity)
	}
	return out
}
