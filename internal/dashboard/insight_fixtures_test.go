package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// note records a Warning event about id, last seen ago before
// testEpoch, count times.
func (tm *testModel) note(
	id inventory.EntityID, reason, message string, count int,
	ago time.Duration,
) {
	tm.noteAt(id, inventory.Note{Reason: reason, Message: message,
		Count: count, Warning: true, At: testEpoch.Add(-ago),
		Source: "kubelet", Origin: reason + message})
}

func (tm *testModel) noteAt(id inventory.EntityID, n inventory.Note) {
	tm.apply(inventory.Observation{Kind: inventory.Noted, Entity: id,
		At: n.At, Note: n})
}

// pod adds a pod of root named name, with attrs.
func (tm *testModel) pod(
	root inventory.EntityID, name string,
	attrs map[string]inventory.Value,
) inventory.EntityID {
	pod := inventory.CoreID(kube.KindPod, root.Namespace, name)
	tm.observe(pod, attrs)
	tm.relate(pod, inventory.OwnedBy, root)
	return pod
}

// failingWith is an incident on root opened an hour ago whose members
// fail with reason on each entity.
func failingWith(
	id string, root inventory.EntityID, reason string,
	entities ...inventory.EntityID,
) incident.Incident {
	p := failingAt(id, root, testEpoch.Add(-time.Hour))
	p.Members = map[detection.Key]detection.Finding{}
	for _, e := range entities {
		f := detection.Finding{Entity: e, Reason: reason,
			Since: p.Opened}
		p.Members[f.Key()] = f
	}
	return p
}
