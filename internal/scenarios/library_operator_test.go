package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// operatorScenarios are operators and the resources they run: a custom
// resource whose failing condition breaks what it owns, and an operator
// whose crash leaves the resources it manages unreconciled.
func operatorScenarios() []scenario {
	return []scenario{operatorCROwnedFailing(), operatorDownCRsStuck()}
}

// kafkaRoot is group-qualified: KafkaCluster is not built in.
const kafkaRoot = "kafkacluster.kafka.example.com/data/events"

// operatorCROwnedFailing: a KafkaCluster reports Ready=False for an
// invalid broker setting, then the brokers of the StatefulSet it owns
// crash on that setting. The custom resource is the root.
func operatorCROwnedFailing() scenario {
	return scenario{
		expect: expectation{
			Name: "operator-cr-owned-failing",
			Description: "A custom resource reports a failing condition; " +
				"the StatefulSet it owns then crash-loops.",
			Root: kafkaRoot, Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"statefulset/data/events-kafka",
				"deployment/kafka-system/kafka-operator"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			op := c.deployment("kafka-system", "kafka-operator",
				"registry.example.com/kafka-operator:1.4", 1)
			c.list(op.objects())
			c.list(op.pod(0, "n1"))
			cr := operatorKafka(c, "True", "Reconciled", "")
			sts := operatorStatefulSet(c, cr)
			c.list(cr, sts)
			c.list(operatorBroker(c, sts, 0, "n1"),
				operatorBroker(c, sts, 1, "n2"))
			c.after(time.Minute)
			message := "broker config log.retention.hours=-5 is invalid"
			c.update(operatorKafka(c, "False", "InvalidBrokerConfig",
				message))
			c.after(3 * time.Minute)
			crash := "ERROR Exiting Kafka: invalid value -5 for " +
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

// operatorDownCRsStuck: the PostgreSQL operator crash-loops; the two
// PostgresClusters it manages are edited and never observed. The
// operator's Deployment is the root, not the resources.
func operatorDownCRsStuck() scenario {
	return scenario{
		expect: expectation{
			Name: "operator-down-crs-stuck",
			Description: "An operator crash-loops; the custom resources " +
				"it manages stop being reconciled.",
			Root: "deployment/db-operators/pg-operator", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{
				"postgrescluster.postgres.example.com/data/orders-db",
				"postgrescluster.postgres.example.com/data/users-db"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			op := c.deployment("db-operators", "pg-operator",
				"registry.example.com/pg-operator:2.5", 1)
			c.list(op.objects())
			c.list(op.pod(0, "n1"))
			names := []string{"orders-db", "users-db"}
			for _, name := range names {
				c.list(operatorPostgres(c, op, name, 3))
			}
			c.after(time.Minute)
			crash := "panic: runtime error: index out of range [3] " +
				"with length 3"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(op.pod(0, "n1", crashLoop(2, "Error", crash,
					restarts)))
				op.setReady(0)
				c.update(op.objects())
				if restarts == 2 {
					for _, name := range names {
						c.update(operatorPostgres(c, op, name, 4))
					}
				}
				c.after(45 * time.Second)
			}
			c.after(5 * time.Minute)
		},
	}
}

func operatorKafka(
	c *cluster, ready, reason, message string,
) *unstructured.Unstructured {
	meta := clusterMeta(c, "data", "events", "kafka")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kafka.example.com/v1", "kind": "KafkaCluster",
		"spec":   map[string]any{"replicas": int64(2)},
		"status": map[string]any{"observedGeneration": int64(1)},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	clusterCondition(u, "Ready", ready, reason, message, c.now)
	return u
}

// operatorStatefulSet is the broker StatefulSet the KafkaCluster owns.
func operatorStatefulSet(
	c *cluster, owner *unstructured.Unstructured,
) *appsv1.StatefulSet {
	meta := clusterMeta(c, "data", "events-kafka", "sts")
	yes := true
	meta.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "kafka.example.com/v1", Kind: "KafkaCluster",
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
				"registry.example.com/kafka:3.7"),
		},
	}
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	sts.Status.Replicas, sts.Status.ReadyReplicas = 2, 2
	sts.Status.AvailableReplicas, sts.Status.CurrentReplicas = 2, 2
	sts.Status.UpdatedReplicas = 2
	return sts
}

// operatorBroker is broker pod i of the StatefulSet on node.
func operatorBroker(
	c *cluster, sts *appsv1.StatefulSet, i int, node string,
	states ...podState,
) *corev1.Pod {
	yes := true
	name := sts.Name + "-" + string(rune('0'+i))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: sts.Namespace, UID: types.UID(name),
			Labels:            sts.Spec.Template.Labels,
			CreationTimestamp: metav1.NewTime(c.start.Add(-time.Hour)),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "StatefulSet", Name: sts.Name,
				UID: sts.UID, Controller: &yes,
			}},
		},
		Spec: *sts.Spec.Template.Spec.DeepCopy(),
	}
	pod.Spec.NodeName = c.n(node)
	running(c, pod)
	for _, state := range states {
		state(c, pod)
	}
	return pod
}

// operatorPostgres is a PostgresCluster whose spec the operator wrote,
// at generation, observed at generation 3.
func operatorPostgres(
	c *cluster, op *workload, name string, generation int64,
) *unstructured.Unstructured {
	meta := clusterMeta(c, "data", name, "pg")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgres.example.com/v1", "kind": "PostgresCluster",
		"spec":   map[string]any{"instances": int64(2)},
		"status": map[string]any{"observedGeneration": int64(3)},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(generation)
	at := metav1.NewTime(c.start.Add(-time.Hour))
	u.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager: op.deployment.Name, Time: &at,
		Operation: metav1.ManagedFieldsOperationUpdate,
	}})
	clusterCondition(u, "Ready", "True", "Reconciled", "",
		c.start.Add(-time.Hour))
	return u
}
