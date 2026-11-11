package scope

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func ownedBy(m *inventory.Model, owned, owner inventory.EntityID) {
	_, _ = m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: maintenanceNow,
		Entity: owned, Relation: inventory.OwnedBy,
		Targets: []inventory.EntityID{owner},
	})
}

// A Node under maintenance holds the static pods it owns.
func TestMaintenanceOnANodeHoldsItsStaticPods(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	node := inventory.CoreID(kube.KindNode, "", "node-1")
	pod := inventory.CoreID(kube.KindPod, "kube-system", "etcd-node-1")
	observe(m, node, held("true"))
	observe(m, pod, nil)
	ownedBy(m, pod, node)
	s := maintenanceScope(t, true)

	assert.False(t, s.Allows(m, detection.Finding{
		Entity: kube.ContainerID("kube-system", "etcd-node-1", "etcd"),
		Reason: "CrashLoopBackOff"}))
}

// A cluster-scoped custom resource under maintenance holds the pods of
// the workload it owns, though the owner reference was filed under the
// workload's namespace.
func TestMaintenanceOnAClusterScopedOwnerHoldsItsPods(t *testing.T) {
	const group = "operator.example.com"
	m := inventory.NewModel(inventory.Options{})
	install := inventory.NewEntityID(group, "install", "", "default")
	filed := inventory.NewEntityID(group, "install", "net", "default")
	ds := inventory.CoreID(kube.KindDaemonSet, "net", "agent")
	pod := inventory.CoreID(kube.KindPod, "net", "agent-x")
	observe(m, install, held("true"))
	observe(m, ds, nil)
	observe(m, pod, nil)
	ownedBy(m, pod, ds)
	ownedBy(m, ds, filed)
	s := maintenanceScope(t, true)
	sig := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff"}

	assert.False(t, s.Allows(m, sig))

	// Not held: the same pod is in scope.
	observe(m, install, held("false"))
	assert.True(t, s.Allows(m, sig))
}
