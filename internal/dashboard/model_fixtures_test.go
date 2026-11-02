package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// testModel is a live inventory for the incident page's context.
type testModel struct {
	t *testing.T
	m *inventory.Model
}

func newTestModel(t *testing.T) *testModel {
	return &testModel{t: t, m: inventory.NewModel(inventory.Options{})}
}

func (tm *testModel) apply(o inventory.Observation) {
	tm.t.Helper()
	if o.Source == "" {
		o.Source = "test"
	}
	if o.At.IsZero() {
		o.At = testEpoch.Add(-2 * time.Hour)
	}
	if _, err := tm.m.Apply(o); err != nil {
		tm.t.Fatal(err)
	}
}

func (tm *testModel) observe(
	id inventory.EntityID, attrs map[string]inventory.Value,
) {
	tm.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: attrs})
}

func (tm *testModel) relate(
	from inventory.EntityID, rel inventory.RelationType,
	to ...inventory.EntityID,
) {
	tm.apply(inventory.Observation{Kind: inventory.Related, Entity: from,
		Relation: rel, Targets: to})
}

func (tm *testModel) change(id inventory.EntityID, c inventory.Change) {
	c.Entity = id
	tm.apply(inventory.Observation{Kind: inventory.Changed, Entity: id,
		At: c.At, Change: c})
}

// deployment is the root "api" in shop with one pod, "api-1".
func (tm *testModel) deployment() (root, pod inventory.EntityID) {
	root = inventory.CoreID(kube.KindDeployment, "shop", "api")
	pod = inventory.CoreID(kube.KindPod, "shop", "api-1")
	tm.observe(root, nil)
	tm.observe(pod, nil)
	tm.relate(pod, inventory.OwnedBy, root)
	return root, pod
}

// container adds a container to pod with attrs.
func (tm *testModel) container(
	pod inventory.EntityID, name string,
	attrs map[string]inventory.Value,
) inventory.EntityID {
	id := kube.ContainerID(pod.Namespace, pod.Name, name)
	tm.observe(id, attrs)
	tm.relate(id, inventory.PartOf, pod)
	return id
}

// contextServer serves items with the model and change history set.
func contextServer(
	m inventory.HistoryReader, changes bool, items ...incident.Incident,
) *Server {
	s := NewServer(Options{Token: testToken, ChangeHistory: changes},
		clock.Func(func() time.Time { return testEpoch }))
	s.SetIncidents(&fakeIncidents{items: items})
	s.SetModel(m)
	return s
}

// detailOf reads id's detail from s.
func detailOf(t *testing.T, s *Server, id string) IncidentDetail {
	t.Helper()
	w := get(t, s, "/api/incidents/"+id)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var d IncidentDetail
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}
