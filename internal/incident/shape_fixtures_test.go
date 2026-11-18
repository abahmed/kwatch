package incident

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// shapeModel is a model the shape tests build by hand.
type shapeModel struct {
	t *testing.T
	m *inventory.Model
}

func newShapeModel(t *testing.T) *shapeModel {
	return &shapeModel{t: t, m: inventory.NewModel(inventory.Options{})}
}

func (s *shapeModel) set(id inventory.EntityID,
	attrs map[string]inventory.Value,
) {
	s.t.Helper()
	_, err := s.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: t0, Entity: id, Attributes: attrs})
	require.NoError(s.t, err)
}

func (s *shapeModel) relate(from inventory.EntityID,
	rel inventory.RelationType, to ...inventory.EntityID,
) {
	s.t.Helper()
	_, err := s.m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: t0, Entity: from, Relation: rel, Targets: to})
	require.NoError(s.t, err)
}

// node adds a node with an architecture label and an instance type.
func (s *shapeModel) node(name, arch, instance string) inventory.EntityID {
	id := inventory.CoreID(kube.KindNode, "", name)
	s.set(id, map[string]inventory.Value{
		kube.AttrReady:        inventory.Bool(true),
		kube.AttrNodeLabels:   inventory.Text("kubernetes.io/arch=" + arch),
		kube.AttrInstanceType: inventory.Text(instance),
	})
	return id
}

// pod adds a pod of owner (a ReplicaSet) on node, ready or not.
func (s *shapeModel) pod(
	name string, owner, node inventory.EntityID, ready bool,
) inventory.EntityID {
	id := entity(kube.KindPod, name)
	s.set(id, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(ready),
		kube.AttrQoS:   inventory.Text("Burstable"),
	})
	s.relate(id, inventory.OwnedBy, owner)
	s.relate(id, inventory.RunsOn, node)
	return id
}

// replicaSet adds a ReplicaSet owned by deploy.
func (s *shapeModel) replicaSet(
	name string, deploy inventory.EntityID,
) inventory.EntityID {
	id := entity(kube.KindReplicaSet, name)
	s.set(id, map[string]inventory.Value{})
	s.relate(id, inventory.OwnedBy, deploy)
	return id
}

// slice gives svc an EndpointSlice with ready of total endpoints.
func (s *shapeModel) slice(svc inventory.EntityID, ready, total int) {
	id := entity(kube.KindEndpointSlice, svc.Name+"-x")
	s.set(id, map[string]inventory.Value{
		kube.AttrEndpointsReady: inventory.Number(float64(ready)),
		kube.AttrEndpoints:      inventory.Number(float64(total)),
	})
	s.relate(id, inventory.Backs, svc)
}

// ingress publishes host/ for svc.
func (s *shapeModel) ingress(name, host string, svc inventory.EntityID) {
	s.set(entity(kube.KindIngress, name), map[string]inventory.Value{
		kube.AttrPublicAddresses: inventory.Text(
			"https\t" + host + "\t/\t" + svc.Name + "\t80"),
	})
}

// crashIncident is an incident rooted at root whose members crash.
func crashIncident(
	root inventory.EntityID, pods ...inventory.EntityID,
) Incident {
	p := Incident{Root: root, Members: map[detection.Key]detection.Finding{}}
	for _, pod := range pods {
		f := sig(pod, reasons.CrashLoopBackOff, detection.Warning)
		p.Members[f.Key()] = f
	}
	return p
}
