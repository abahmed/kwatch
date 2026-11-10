package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// busRoot is the cluster-scoped custom resource: it has no namespace.
const busRoot = "messagebus.bus.example.com//events"

// operatorClusterCROwnedFailing is operatorCROwnedFailing with an owner
// that has no namespace: a cluster-scoped MessageBus reports
// Ready=False for an invalid broker setting, then the brokers of the
// StatefulSet it owns in the data namespace crash on that setting. An
// owner reference names no namespace, so the owner must be found as
// the cluster-scoped resource it is, not looked for (and missed) in
// the namespace of the StatefulSet.
func operatorClusterCROwnedFailing() scenario {
	return scenario{
		expect: expectation{
			Name: "operator-cluster-cr-owned-failing",
			Description: "A cluster-scoped custom resource reports a " +
				"failing condition; the StatefulSet it owns in a " +
				"namespace then crash-loops.",
			Root: busRoot, Tier: "notify", MaxMessages: 1,
			MustNotBlame: []string{"statefulset/data/events-bus",
				"deployment/bus-system/bus-operator"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			op := c.deployment("bus-system", "bus-operator",
				"registry.example.com/bus-operator:1.4", 1)
			c.list(op.objects())
			c.list(op.pod(0, "n1"))
			cr := operatorBus(c, "True", "Reconciled", "")
			sts := operatorBusStatefulSet(c, cr)
			c.list(cr, sts)
			c.list(operatorBroker(c, sts, 0, "n1"),
				operatorBroker(c, sts, 1, "n2"))
			c.after(time.Minute)
			message := "broker config log.retention.hours=-5 is invalid"
			c.update(operatorBus(c, "False", "InvalidBrokerConfig",
				message))
			c.after(3 * time.Minute)
			crash := "ERROR Exiting broker: invalid value -5 for " +
				"configuration log.retention.hours"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(operatorBroker(c, sts, 0, "n1",
					crashLoop(1, "Error", crash, restarts)),
					operatorBroker(c, sts, 1, "n2",
						crashLoop(1, "Error", crash, restarts)))
				c.after(45 * time.Second)
			}
		},
	}
}

// operatorBus is the cluster-scoped MessageBus named events.
func operatorBus(
	c *cluster, ready, reason, message string,
) *unstructured.Unstructured {
	meta := clusterMeta(c, "", "events", "bus")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "bus.example.com/v1", "kind": "MessageBus",
		"spec":   map[string]any{"replicas": int64(2)},
		"status": map[string]any{"observedGeneration": int64(1)},
	}}
	u.SetName(meta.Name)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	clusterCondition(u, "Ready", ready, reason, message, c.now)
	return u
}

// operatorBusStatefulSet is the broker StatefulSet the MessageBus owns.
func operatorBusStatefulSet(
	c *cluster, owner *unstructured.Unstructured,
) *appsv1.StatefulSet {
	meta := clusterMeta(c, "data", "events-bus", "sts")
	yes := true
	meta.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "bus.example.com/v1", Kind: "MessageBus",
		Name: owner.GetName(), UID: owner.GetUID(), Controller: &yes,
	}}
	replicas := int32(2)
	labels := map[string]string{"app": meta.Name}
	sts := &appsv1.StatefulSet{
		ObjectMeta: meta,
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels,
				"registry.example.com/bus:3.7"),
		},
	}
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	sts.Status.Replicas, sts.Status.ReadyReplicas = 2, 2
	sts.Status.AvailableReplicas, sts.Status.CurrentReplicas = 2, 2
	sts.Status.UpdatedReplicas = 2
	return sts
}
