package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ownerModel builds a model from present entities and owned-by pairs
// (owned, owner).
func ownerModel(
	t *testing.T, present []EntityID, owned ...[2]EntityID,
) *Model {
	t.Helper()
	m := NewModel(Options{})
	for _, id := range present {
		_, err := m.Apply(Observation{Kind: Observed, Source: "test",
			At: testTime, Entity: id})
		assert.NoError(t, err)
	}
	for _, pair := range owned {
		_, err := m.Apply(Observation{Kind: Related, Source: "test",
			At: testTime, Entity: pair[0], Relation: OwnedBy,
			Targets: []EntityID{pair[1]}})
		assert.NoError(t, err)
	}
	return m
}

// A static pod is its own workload: the Node that owns it is where it
// runs. Two static pods of one node are two workloads, so they do not
// share a baseline.
func TestTopOwnerStopsAtTheNodeOfAStaticPod(t *testing.T) {
	node := CoreID("node", "", "node-1")
	etcd := CoreID("pod", "kube-system", "etcd-node-1")
	api := CoreID("pod", "kube-system", "apiserver-node-1")
	m := ownerModel(t, []EntityID{node, etcd, api},
		[2]EntityID{etcd, node}, [2]EntityID{api, node})

	assert.Equal(t, etcd, TopOwner(m, etcd))
	assert.Equal(t, api, TopOwner(m, api))
	owner, ok := Owner(m, etcd)
	assert.True(t, ok)
	assert.Equal(t, node, owner, "the node is still the pod's owner")
}

// The owner of a pod was filed under the pod's namespace, but it is a
// cluster-scoped custom resource: the one that exists is the owner.
func TestOwnerFallsBackToTheClusterScopedOwner(t *testing.T) {
	const group = "operator.example.com"
	filed := NewEntityID(group, "install", "net", "default")
	real := NewEntityID(group, "install", "", "default")
	ds := CoreID("daemonset", "net", "agent")
	pod := CoreID("pod", "net", "agent-x")
	m := ownerModel(t, []EntityID{real, ds, pod},
		[2]EntityID{pod, ds}, [2]EntityID{ds, filed})

	owner, ok := Owner(m, ds)
	assert.True(t, ok)
	assert.Equal(t, real, owner)
	assert.Equal(t, real, TopOwner(m, pod))
	_, found := m.Entity(TopOwner(m, pod))
	assert.True(t, found, "the top owner is an entity that exists")
}

// A namespaced owner that exists wins over a cluster-scoped namesake,
// and an owner that exists nowhere is returned as it was filed.
func TestOwnerKeepsTheNamespacedOwner(t *testing.T) {
	const group = "operator.example.com"
	namespaced := NewEntityID(group, "install", "net", "default")
	cluster := NewEntityID(group, "install", "", "default")
	ds := CoreID("daemonset", "net", "agent")
	m := ownerModel(t, []EntityID{namespaced, cluster, ds},
		[2]EntityID{ds, namespaced})
	owner, _ := Owner(m, ds)
	assert.Equal(t, namespaced, owner)

	missing := ownerModel(t, []EntityID{ds}, [2]EntityID{ds, namespaced})
	owner, ok := Owner(missing, ds)
	assert.True(t, ok)
	assert.Equal(t, namespaced, owner)

	_, ok = Owner(missing, namespaced)
	assert.False(t, ok, "no owner")
}
