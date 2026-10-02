package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// clusterRegistry is the private registry of the image scenarios. It is
// shared by the whole cluster, so it is never suffixed.
const clusterRegistry = "registry.corp.example"

// clusterRegistryAuth: the pull credential for the private registry has
// expired; replicas scheduled onto a new node cannot pull three different
// images, while replicas with cached images keep running. The registry is
// the root.
func clusterRegistryAuth() scenario {
	return scenario{
		expect: expectation{
			Name: "registry-auth-failure",
			Description: "Pods of three workloads scheduled onto a new " +
				"node fail to pull from one private registry with 401 " +
				"Unauthorized; no rollout happened.",
			Root: "registry//" + clusterRegistry, Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n3", "deployment/shop/api",
				"deployment/shop/worker", "deployment/billing/ledger"},
		},
		build: buildClusterRegistryAuth,
	}
}

func buildClusterRegistryAuth(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	apps := []*workload{
		c.deployment("shop", "api", clusterRegistry+"/shop/api:4.2", 2),
		c.deployment("shop", "worker", clusterRegistry+"/shop/worker:4.2",
			2),
		c.deployment("billing", "ledger",
			clusterRegistry+"/billing/ledger:1.7", 2),
	}
	for _, w := range apps {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(time.Minute)
	c.create(c.node("n3", "zone-a"))
	for _, w := range apps {
		setReplicas(w, 3)
		w.setReady(2)
		c.update(w.objects())
	}
	for n := range 6 {
		for _, w := range apps {
			clusterPullFailure(c, w, 2, "n3", n,
				"failed to authorize: failed to fetch oauth token: "+
					"unexpected status from GET request to https://"+
					clusterRegistry+"/service/token: 401 Unauthorized")
		}
		c.after(40 * time.Second)
	}
}

// clusterPullFailure moves replica i of w through the kubelet's pull
// cycle: ErrImagePull with the registry's answer, then back-off. Each
// failed attempt is also a Failed Warning event.
func clusterPullFailure(
	c *cluster, w *workload, i int, node string, attempt int, cause string,
) {
	image := w.deployment.Spec.Template.Spec.Containers[0].Image
	detail := "failed to pull and unpack image \"" + image + "\": " +
		"failed to resolve reference \"" + image + "\": " + cause
	state := waiting("ErrImagePull", detail)
	if attempt%2 == 1 {
		state = waiting("ImagePullBackOff",
			"Back-off pulling image \""+image+"\"")
	}
	pod := w.pod(i, node, startedNow, state)
	pod.CreationTimestamp = metav1.NewTime(c.now.Add(
		-time.Duration(attempt) * 40 * time.Second))
	c.update(pod)
	if attempt%2 == 0 {
		c.warn(c.warningEvent(pod, "Pod", "Failed",
			"Failed to pull image \""+image+"\": "+detail, "kubelet",
			int32(attempt/2+1)))
	}
}

// clusterImageTypo: one workload rolls out a tag that does not exist;
// other workloads pull from the same registry fine. The rollout of that
// Deployment is the root, never the registry.
func clusterImageTypo() scenario {
	return scenario{
		expect: expectation{
			Name: "image-typo",
			Description: "A Deployment rolls out a mistyped image tag " +
				"that the registry does not have; other workloads from " +
				"the same registry stay healthy.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"registry//" + clusterRegistry},
		},
		build: buildClusterImageTypo,
	}
}

func buildClusterImageTypo(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	checkout := c.deployment("shop", "checkout",
		clusterRegistry+"/shop/checkout:1.9.0", 2)
	others := []*workload{
		c.deployment("shop", "catalog", clusterRegistry+"/shop/catalog:3.3",
			2),
		c.deployment("shop", "search", clusterRegistry+"/shop/search:2.0",
			2),
	}
	for _, w := range append([]*workload{checkout}, others...) {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(time.Minute)
	image := clusterRegistry + "/shop/checkout:1.9.O"
	rs := checkout.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = image
	})
	checkout.setReady(2)
	c.update(checkout.deployment)
	c.create(rs)
	for n := range 6 {
		clusterPullFailure(c, checkout, 0, "n1", n, image+": not found")
		c.after(40 * time.Second)
	}
}

// clusterNetworkPolicyChange: a new egress policy selects the api pods
// and only allows DNS; they lose their database and fail readiness, while
// the worker it does not select stays healthy. The policy is the root.
func clusterNetworkPolicyChange() scenario {
	return scenario{
		expect: expectation{
			Name: "networkpolicy-change",
			Description: "A NetworkPolicy restricting egress to DNS is " +
				"created for the api pods; they time out connecting to " +
				"their database while unselected pods stay healthy.",
			Root: "networkpolicy/payments/restrict-egress", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"cluster-dns//cluster-dns"},
		},
		build: buildClusterNetworkPolicy,
	}
}

func buildClusterNetworkPolicy(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	api := c.deployment("payments", "api", "registry.example.com/pay:6", 2)
	worker := c.deployment("payments", "worker",
		"registry.example.com/pay-worker:6", 2)
	for _, w := range []*workload{api, worker} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(time.Minute)
	c.create(clusterEgressPolicy(c, api))
	c.after(20 * time.Second)
	message := "FATAL: failed to connect to `host=10.0.3.4 user=payments " +
		"database=payments`: dial error (dial tcp 10.0.3.4:5432: i/o " +
		"timeout)"
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(api.pod(0, "n1", crashLoop(1, "Error", message, restarts)),
			api.pod(1, "n2", crashLoop(1, "Error", message, restarts)))
		api.setReady(0)
		c.update(api.objects())
		c.after(45 * time.Second)
	}
}

func clusterEgressPolicy(
	c *cluster, w *workload,
) *networkingv1.NetworkPolicy {
	udp, tcp := corev1.ProtocolUDP, corev1.ProtocolTCP
	dns := intstr.FromInt32(53)
	return &networkingv1.NetworkPolicy{
		ObjectMeta: clusterMeta(c, "payments", "restrict-egress", "netpol"),
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: w.deployment.Spec.Template.Labels,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &udp, Port: &dns},
					{Protocol: &tcp, Port: &dns},
				},
			}},
		},
	}
}

// clusterOperatorCRStuck: a PostgresCluster spec edit asks to shrink its
// volume; the healthy operator refuses and the resource stays not ready
// at its old generation. The edited custom resource is the root, not the
// operator.
// operatorCRRoot is group-qualified: PostgresCluster is not built in.
const operatorCRRoot = "postgrescluster.postgres.example.com/data/orders-db"

func clusterOperatorCRStuck() scenario {
	return scenario{
		expect: expectation{
			Name: "operator-cr-stuck",
			Description: "A custom resource's spec is changed to an " +
				"invalid value; its operator stays healthy but reports " +
				"Ready=False with ReconcileError and never observes the " +
				"new generation.",
			Root: operatorCRRoot, Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"deployment/db-operators/pg-operator"},
		},
		build: buildClusterOperatorCR,
	}
}

func buildClusterOperatorCR(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	operator := c.deployment("db-operators", "pg-operator",
		"registry.example.com/pg-operator:2.4", 1)
	c.list(operator.objects())
	c.list(operator.pod(0, "n1"))
	c.list(clusterPostgres(c, 3, "100Gi", 3, "True", "Reconciled", ""))
	c.after(time.Minute)
	message := "spec.storage.size: cannot shrink volume from 100Gi to " +
		"50Gi"
	since := c.now
	for n := range 6 {
		cr := clusterPostgres(c, 4, "50Gi", 3, "False", "ReconcileError",
			message)
		clusterCondition(cr, "Ready", "False", "ReconcileError", message,
			since)
		c.update(cr)
		if n%2 == 0 {
			c.warn(c.warningEvent(cr, "PostgresCluster", "ReconcileError",
				message, "pg-operator", int32(n/2+1)))
		}
		c.after(45 * time.Second)
	}
}

func clusterPostgres(
	c *cluster, generation int64, size string, observed int64,
	ready, reason, message string,
) *unstructured.Unstructured {
	meta := clusterMeta(c, "data", "orders-db", "pg")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgres.example.com/v1", "kind": "PostgresCluster",
		"spec": map[string]any{
			"instances": int64(2),
			"storage":   map[string]any{"size": size},
		},
		"status": map[string]any{"observedGeneration": observed},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(generation)
	clusterCondition(u, "Ready", ready, reason, message, c.now)
	return u
}

func clusterCondition(
	u *unstructured.Unstructured, kind, status, reason, message string,
	since time.Time,
) {
	_ = unstructured.SetNestedSlice(u.Object, []any{map[string]any{
		"type": kind, "status": status, "reason": reason,
		"message":            message,
		"lastTransitionTime": since.UTC().Format(time.RFC3339),
	}}, "status", "conditions")
}
