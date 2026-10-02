package explain

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// t0 is when every fixture starts; failures begin a few minutes later.
var t0 = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

// fixture builds a small inventory graph with findings, changes and
// links, then solves it.
type fixture struct {
	t        *testing.T
	model    *inventory.Model
	findings map[inventory.EntityID][]detection.Finding
	changes  fakeChanges
	links    fakeLinks
	baseline fakeBaseline
	now      time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return &fixture{
		t: t, model: inventory.NewModel(inventory.Options{}),
		findings: map[inventory.EntityID][]detection.Finding{},
		changes:  fakeChanges{}, links: fakeLinks{},
		now: t0.Add(10 * time.Minute),
	}
}

func (f *fixture) apply(o inventory.Observation) {
	f.t.Helper()
	o.Source, o.At = "test", t0
	if _, err := f.model.Apply(o); err != nil {
		f.t.Fatal(err)
	}
}

// add observes entities without attributes.
func (f *fixture) add(ids ...inventory.EntityID) {
	for _, id := range ids {
		f.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
			Attributes: map[string]inventory.Value{}})
	}
}

// relate stores from → relation → targets.
func (f *fixture) relate(
	from inventory.EntityID, relation inventory.RelationType,
	targets ...inventory.EntityID,
) {
	f.apply(inventory.Observation{Kind: inventory.Related, Entity: from,
		Relation: relation, Targets: targets})
}

// note records an event message about id.
func (f *fixture) note(id inventory.EntityID, message string) {
	f.apply(inventory.Observation{Kind: inventory.Noted, Entity: id,
		Note: inventory.Note{At: t0.Add(time.Minute), Source: "kubelet",
			Reason: "Failed", Message: message, Warning: true}})
}

// fail gives id a finding in mode, starting minutes after t0.
func (f *fixture) fail(
	id inventory.EntityID, mode detection.Mode, health detection.Health,
	minutes int, text string,
) {
	f.findings[id] = append(f.findings[id], detection.Finding{
		Entity: id, Reason: string(mode), Mode: mode, Health: health,
		Since:   t0.Add(time.Duration(minutes) * time.Minute),
		Summary: string(mode), Evidence: []detection.Evidence{
			{Label: "message", Value: text}},
	})
}

// change records a change of id minutes after t0.
func (f *fixture) change(
	id inventory.EntityID, minutes int, paths ...string,
) {
	change := inventory.Change{Entity: id,
		At: t0.Add(time.Duration(minutes) * time.Minute)}
	for _, path := range paths {
		change.Fields = append(change.Fields, inventory.FieldChange{
			Path: path, After: "x"})
	}
	change.Created = len(paths) == 0
	f.changes[id] = append(f.changes[id], change)
}

// workload adds a Deployment with one ReplicaSet and n pods on the
// given nodes (round robin); each pod has one container "app".
func (f *fixture) workload(
	namespace, name string, n int, nodes ...inventory.EntityID,
) []inventory.EntityID {
	deployment := inventory.CoreID(kube.KindDeployment, namespace, name)
	rs := inventory.CoreID(kube.KindReplicaSet, namespace, name+"-1")
	f.add(deployment, rs)
	f.relate(rs, inventory.OwnedBy, deployment)
	pods := make([]inventory.EntityID, 0, n)
	for i := 0; i < n; i++ {
		pod := inventory.CoreID(kube.KindPod, namespace,
			fmt.Sprintf("%s-1-%d", name, i))
		f.add(pod, containerOf(pod))
		f.relate(pod, inventory.OwnedBy, rs)
		f.relate(containerOf(pod), inventory.PartOf, pod)
		if len(nodes) > 0 {
			f.relate(pod, inventory.RunsOn, nodes[i%len(nodes)])
		}
		pods = append(pods, pod)
	}
	return pods
}

// nodes adds nodes in a zone.
func (f *fixture) nodes(zone string, names ...string) []inventory.EntityID {
	z := inventory.CoreID(kube.KindZone, "", zone)
	var out []inventory.EntityID
	for _, name := range names {
		node := inventory.CoreID(kube.KindNode, "", name)
		f.add(node)
		f.relate(node, inventory.PartOf, z)
		out = append(out, node)
	}
	return out
}

func containerOf(pod inventory.EntityID) inventory.EntityID {
	return inventory.CoreID(kube.KindContainer, pod.Namespace,
		pod.Name+"/app")
}

func (f *fixture) snapshot() Snapshot {
	return Snapshot{
		Model: f.model, Findings: f.findings, Links: f.links,
		Changes: f.changes, Baseline: f.baseline,
		Synced: func(inventory.Kind) bool { return true }, Now: f.now,
	}
}

func (f *fixture) explain() Explanation {
	return Explain(f.snapshot())
}

// causes lists the roots of every stated cause.
func causes(e Explanation) []string {
	var out []string
	for _, area := range e.Areas {
		for _, c := range area.Causes {
			out = append(out, c.Root.String())
		}
	}
	return out
}

// requireCause fails unless want is a stated cause covering id.
func requireCause(
	t *testing.T, e Explanation, id inventory.EntityID, want string,
) Cause {
	t.Helper()
	c, ok := e.CauseOf(id)
	if !ok || c.Root.String() != want {
		t.Fatalf("cause of %s = %v (ok %v), want %s; all causes %v", id,
			c.Root, ok, want, causes(e))
	}
	requireCoded(t, c)
	return c
}

// requireCoded checks every contribution of a cause carries a code:
// writers word evidence from its code, never from its text.
func requireCoded(t *testing.T, c Cause) {
	t.Helper()
	for _, contribution := range c.Contributions {
		if contribution.Code == "" {
			t.Errorf("contribution of %s has no code: %+v",
				contribution.Scorer, contribution)
		}
	}
}

// fakeChanges is a ChangeReader over a map.
type fakeChanges map[inventory.EntityID][]inventory.Change

func (c fakeChanges) Changes(
	id inventory.EntityID, since, until time.Time,
) []inventory.Change {
	var out []inventory.Change
	for _, change := range c[id] {
		if !change.At.Before(since) && !change.At.After(until) {
			out = append(out, change)
		}
	}
	return out
}

// fakeLinks is a LinkReader over a map.
type fakeLinks map[inventory.EntityID][]Link

func (l fakeLinks) Links(id inventory.EntityID) []Link { return l[id] }

// fakeBaseline is a BaselineReader over a map; nil reads as neutral.
type fakeBaseline map[inventory.EntityID]float64

func (b fakeBaseline) Deviation(id inventory.EntityID) (float64, bool) {
	d, ok := b[id]
	return d, ok
}

// scoreOf runs one scorer on the candidate root of a fixture, after
// the same candidate and ranking steps Explain runs.
func scoreOf(
	t *testing.T, f *fixture, root inventory.EntityID,
	score func(*view, *candidate) outcome,
) outcome {
	t.Helper()
	return scoreSnapshot(t, f.snapshot(), root, score)
}

func scoreSnapshot(
	t *testing.T, s Snapshot, root inventory.EntityID,
	score func(*view, *candidate) outcome,
) outcome {
	t.Helper()
	v := newView(s)
	cs := v.candidates(failures(s.Findings))
	v.rank(cs)
	c, ok := cs.byID[root]
	if !ok {
		t.Fatalf("%s is not a candidate", root)
	}
	return score(v, c)
}

// requireWeight fails unless the outcome has the given weight.
func requireWeight(t *testing.T, o outcome, want float64) {
	t.Helper()
	if diff := o.weight - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("weight = %v (%q), want %v", o.weight, o.text, want)
	}
}

// nodeWithPods puts n pods of each named workload on n1 and one
// replica of each on n2, and returns n1 and the pods on n1.
func nodeWithPods(
	f *fixture, n int, workloads ...string,
) (inventory.EntityID, []inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1", "n2")
	var onNode []inventory.EntityID
	for _, name := range workloads {
		pods := f.workload("shop", name, n+1, nodes[0])
		f.relate(pods[n], inventory.RunsOn, nodes[1])
		onNode = append(onNode, pods[:n]...)
	}
	return nodes[0], onNode
}
