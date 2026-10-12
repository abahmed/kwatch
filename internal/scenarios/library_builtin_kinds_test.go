package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// builtinKindScenarios are failures of built-in kinds that are not
// workloads: certificate requests, API priority and fairness objects,
// device claims, disruption budgets and StatefulSet rollouts. An
// approved certificate request that is never issued has no scenario: its
// detector waits five minutes, which with the notify settle is beyond the
// five-minute first-message gate by design (the detector test covers it).
func builtinKindScenarios() []scenario {
	return []scenario{
		csrDenied(), flowSchemaDangling(),
		resourceClaimUnallocated(), budgetsOverlapEvictionRefused(),
		statefulSetRolloutStuck(),
	}
}

// builtinObject is a cluster-scoped or namespaced object of a built-in
// API group, read the way the dynamic watch reads it.
func builtinObject(c *cluster, apiVersion, kind, namespace, name string,
	spec, status map[string]any,
) *unstructured.Unstructured {
	meta := clusterMeta(c, namespace, name, kind)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion, "kind": kind,
		"spec": spec, "status": status,
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	return u
}

func csrCondition(c *cluster, kind, reason, message string) []any {
	return []any{map[string]any{
		"type": kind, "status": "True", "reason": reason,
		"message":            message,
		"lastTransitionTime": c.now.UTC().Format(time.RFC3339),
	}}
}

// csrDenied: the request a node made for its client certificate was
// denied, so the node cannot join the cluster.
func csrDenied() scenario {
	return scenario{
		expect: expectation{
			Name: "csr-denied",
			Description: "A certificate signing request for a node's " +
				"client certificate is denied.",
			Root: "certificatesigningrequest//csr-w7k2p", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.after(time.Minute)
			c.create(builtinObject(c, "certificates.k8s.io/v1",
				"CertificateSigningRequest", "", "csr-w7k2p",
				map[string]any{"signerName": "kubernetes.io/kube-" +
					"apiserver-client-kubelet"},
				map[string]any{"conditions": csrCondition(c, "Denied",
					"AutoDenied", "node identity does not match")}))
		},
	}
}

// flowSchemaDangling: a FlowSchema references a priority level that does
// not exist, so requests matching it fall to a catch-all level.
func flowSchemaDangling() scenario {
	return scenario{
		expect: expectation{
			Name: "flowschema-dangling-priority-level",
			Description: "A FlowSchema reports Dangling because the " +
				"priority level it names does not exist.",
			Root: "flowschema//batch-jobs",
			Tier: "notify", MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			u := builtinObject(c, "flowcontrol.apiserver.k8s.io/v1",
				"FlowSchema", "", "batch-jobs",
				map[string]any{"priorityLevelConfiguration": map[string]any{
					"name": "batch-low"}},
				map[string]any{"conditions": csrCondition(c, "Dangling",
					"NotFound", "this FlowSchema references a "+
						"non-existent PriorityLevelConfiguration")})
			c.create(u)
			c.after(5 * time.Minute)
		},
	}
}

// resourceClaimUnallocated: a pod's ResourceClaim got no device, so the
// pod that owns it cannot start.
func resourceClaimUnallocated() scenario {
	return scenario{
		expect: expectation{
			Name: "resourceclaim-unallocated-pod-pending",
			Description: "A pod waits for a device claim the scheduler " +
				"could not allocate.",
			Root: "resourceclaim/ml/gpu-claim",
			Tier: "notify", MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("ml", "trainer",
				"registry.example.com/trainer:2", 1)
			c.list(w.objects())
			pod := w.pod(0, "", pendingUnscheduled("0/1 nodes are "+
				"available: 1 cannot allocate all claims."))
			c.list(pod)
			yes := true
			claim := builtinObject(c, "resource.k8s.io/v1", "ResourceClaim",
				"ml", "gpu-claim", map[string]any{}, map[string]any{})
			claim.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: "v1", Kind: "Pod", Name: pod.Name,
				UID: pod.UID, Controller: &yes}})
			c.list(claim)
			c.after(15 * time.Minute)
		},
	}
}

// budgetsOverlapEvictionRefused: two disruption budgets select the same
// pods, and the disruption controller says it refuses to evict them.
func budgetsOverlapEvictionRefused() scenario {
	return scenario{
		expect: expectation{
			Name: "pdb-overlap-eviction-refused",
			Description: "Two disruption budgets select the same pods " +
				"and the controller reports that evicting them fails.",
			Root: "poddisruptionbudget/shop/cart-min", Tier: "notify",
			OtherRoots:  []string{"poddisruptionbudget/shop/cart-max"},
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "cart",
				"registry.example.com/cart:5", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n2")}
			c.list(pods[0], pods[1])
			for _, pod := range pods {
				c.warn(c.warningEvent(pod, "Pod",
					"MultiplePodDisruptionBudgets", "This pod has more "+
						"than one PodDisruptionBudget, which the "+
						"eviction subresource does not support.",
					"controllermanager", 3))
			}
			c.after(time.Minute)
			c.list(overlapBudget(c, w, "cart-min"),
				overlapBudget(c, w, "cart-max"))
			c.after(10 * time.Minute)
		},
	}
}

func overlapBudget(c *cluster, w *workload, name string,
) *policyv1.PodDisruptionBudget {
	one := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: clusterMeta(c, "shop", name, "pdb"),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: w.deployment.Spec.Template.Labels},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			CurrentHealthy: 2, DesiredHealthy: 1, ExpectedPods: 2,
			DisruptionsAllowed: 1,
		},
	}
}

// statefulSetRolloutStuck: a StatefulSet is rolled to a new revision;
// ordinal 2, replaced first, never becomes ready, so the update halts.
func statefulSetRolloutStuck() scenario {
	return scenario{
		expect: expectation{
			Name: "statefulset-rollout-stuck",
			Description: "A StatefulSet rolls to a new revision and the " +
				"first replaced pod never becomes ready.",
			Root: "statefulset/streaming/queue", Tier: "notify",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: buildStatefulSetRolloutStuck,
	}
}

func buildStatefulSetRolloutStuck(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	meta := clusterMeta(c, "streaming", "queue", "sts")
	replicas := int32(3)
	labels := map[string]string{"app": meta.Name}
	sts := &appsv1.StatefulSet{
		ObjectMeta: meta,
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas, ServiceName: meta.Name,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, "registry.example.com/queue:4"),
		},
	}
	sts.Generation, sts.Status.ObservedGeneration = 1, 1
	sts.Status.Replicas, sts.Status.ReadyReplicas = 3, 3
	sts.Status.AvailableReplicas, sts.Status.CurrentReplicas = 3, 3
	sts.Status.CurrentRevision, sts.Status.UpdateRevision = "queue-a", "queue-a"
	sts.Status.UpdatedReplicas = 3
	c.list(sts)
	for i := range 3 {
		c.list(operatorBroker(c, sts, i, "n1"))
	}
	c.after(5 * time.Minute)
	rolled := sts.DeepCopy()
	rolled.Generation, rolled.Status.ObservedGeneration = 2, 2
	rolled.Spec.Template.Spec.Containers[0].Image =
		"registry.example.com/queue:5"
	rolled.Status.UpdateRevision = "queue-b"
	rolled.Status.UpdatedReplicas = 1
	rolled.Status.ReadyReplicas, rolled.Status.AvailableReplicas = 2, 2
	c.update(rolled)
	c.update(operatorBroker(c, rolled, 2, "n2", notReady))
	c.after(20 * time.Minute)
}
