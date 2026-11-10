package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func controlledBy(apiVersion, kind, name string) []metav1.OwnerReference {
	controller := true
	return []metav1.OwnerReference{{
		APIVersion: apiVersion, Kind: kind, Name: name,
		Controller: &controller,
	}}
}

// A static pod is "owned" by its Node. A Node has no namespace, so the
// owner is the node itself, not a node filed under the pod's namespace.
func TestStaticPodOwnerIsTheNodeWithoutANamespace(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "etcd-node-1", Namespace: "kube-system",
		OwnerReferences: controlledBy("v1", "Node", "node-1"),
	}}
	d, ok := kube.PodSchema{}.Describe(pod)
	require.True(t, ok)
	assert.Equal(t, []inventory.EntityID{
		inventory.CoreID(kube.KindNode, "", "node-1"),
	}, d.Relations[inventory.OwnedBy])
}

// ownerOf is the owned-by target of a DaemonSet in namespace whose
// controller is a custom resource of kind and name.
func ownerOf(t *testing.T, namespace, kind, name string) inventory.EntityID {
	t.Helper()
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name: "agent", Namespace: namespace,
		OwnerReferences: controlledBy(
			"operator.example.com/v1", kind, name),
	}}
	d, ok := kube.DaemonSetSchema().Describe(ds)
	require.True(t, ok)
	require.Len(t, d.Relations[inventory.OwnedBy], 1)
	return d.Relations[inventory.OwnedBy][0]
}

// A custom owner stays in the namespace of what it owns, even one
// whose kind starts with "Cluster": only a built-in cluster kind drops
// the namespace. inventory.ResolveOwner finds a cluster-scoped custom
// owner when it is read.
func TestOwnerNamespaceFollowsTheOwnerKind(t *testing.T) {
	const group = "operator.example.com"
	assert.Equal(t,
		inventory.NewEntityID(group, "clusterinstall", "net", "default"),
		ownerOf(t, "net", "ClusterInstall", "default"))
	assert.Equal(t,
		inventory.NewEntityID(group, "install", "net", "default"),
		ownerOf(t, "net", "Install", "default"))
}

// A namespaced database operator's Cluster named "main" in two
// namespaces is two owners, not one shared across them.
func TestNamespacedClusterOwnersDoNotMerge(t *testing.T) {
	shop := ownerOf(t, "shop", "Cluster", "main")
	pay := ownerOf(t, "pay", "Cluster", "main")
	assert.Equal(t, "shop", shop.Namespace)
	assert.Equal(t, "pay", pay.Namespace)
	assert.NotEqual(t, shop, pay)
}
