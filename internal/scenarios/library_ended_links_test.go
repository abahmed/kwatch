package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// endedLinkScenarios are dependencies removed just before a failure:
// the model keeps only the links that hold now, so the end of one is
// recorded as a change of the object that held it.
func endedLinkScenarios() []scenario {
	return []scenario{
		{expect: expectation{
			Name: "networkpolicy-stops-covering-workload",
			Description: "Under a default-deny policy, the policy that " +
				"lets the API reach its database is edited so it no " +
				"longer selects the API, which then times out on the " +
				"database.",
			Root: "networkpolicy/shop/orders-egress", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"networkpolicy/shop/deny-all",
				"deployment/shop/postgres", "node//n1"},
		}, build: func(c *cluster) { buildPolicyStopsCovering(c, false) }},
		{expect: expectation{
			Name: "networkpolicy-selector-flips-back",
			Description: "The API's egress policy is edited to a typo " +
				"and set back within half a minute; nothing fails.",
			Quiet: true,
		}, build: func(c *cluster) { buildPolicyStopsCovering(c, true) }},
		{expect: expectation{
			Name: "pod-restart-keeps-links",
			Description: "An API pod is deleted and replaced, and the " +
				"new one becomes ready; its links end with the old pod " +
				"and nothing is said.",
			Quiet: true,
		}, build: buildPodRestartKeepsLinks},
	}
}

// buildPolicyStopsCovering lists the orders API and its database under
// a default-deny policy and an egress policy that lets the API reach
// the database, then edits that policy's selector to a typo. With back
// it is set back 20 seconds later and nothing fails; otherwise the API
// times out on the database.
func buildPolicyStopsCovering(c *cluster, back bool) {
	c.list(c.node("n1", "zone-a"))
	db := c.deployment("shop", "postgres",
		"registry.example.com/postgres:16", 1)
	c.list(db.objects())
	dbPod := db.pod(0, "n1")
	c.list(dbPod, clusterService(c, "shop", "postgres", 5432),
		trafficSlice(c, "postgres", dbPod))
	api := c.deployment("shop", "orders", "registry.example.com/orders:4", 2)
	configTemplate(api, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "DATABASE_URL", Value: "postgres://orders@" +
				c.n("postgres") + ":5432/orders",
		}}
	})
	c.list(api.objects())
	c.list(api.pod(0, "n1"), api.pod(1, "n1"))
	deny := denyAllPolicy(c)
	deny.ManagedFields[0].Time = &metav1.Time{Time: c.now.Add(-time.Hour)}
	allow := ordersEgress(c, api.deployment.Spec.Template.Labels, db)
	c.list(deny, allow)
	c.after(5 * time.Minute)
	c.update(withSelector(c, allow, map[string]string{"app": "order"}))
	if back {
		c.after(20 * time.Second)
		c.update(withSelector(c, allow, api.deployment.Spec.Template.Labels))
		c.after(10 * time.Minute)
		return
	}
	c.after(20 * time.Second)
	message := "FATAL: could not connect to server: Connection " +
		"timed out; dial tcp 10.96.14.3:5432: i/o timeout"
	for restarts := int32(1); restarts <= 5; restarts++ {
		for replica := range 2 {
			c.update(api.pod(replica, "n1", crashLoop(1, "Error",
				message, restarts)))
		}
		api.setReady(0)
		c.update(api.objects())
		c.after(45 * time.Second)
	}
}

// ordersEgress selects the pods labelled selects and lets them reach
// the database's pods on 5432.
func ordersEgress(
	c *cluster, selects map[string]string, db *workload,
) *networkingv1.NetworkPolicy {
	tcp := corev1.ProtocolTCP
	port := intstr.FromInt32(5432)
	return &networkingv1.NetworkPolicy{
		ObjectMeta: clusterMeta(c, "shop", "orders-egress", "netpol"),
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: selects},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: db.deployment.Spec.Template.Labels},
				}},
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &tcp, Port: &port}},
			}},
		},
	}
}

// withSelector is policy with its pod selector set to selects, written
// by bob now.
func withSelector(
	c *cluster, policy *networkingv1.NetworkPolicy,
	selects map[string]string,
) *networkingv1.NetworkPolicy {
	edited := last(c, policy)
	edited.Spec.PodSelector = metav1.LabelSelector{MatchLabels: selects}
	edited.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager: "bob", Operation: metav1.ManagedFieldsOperationUpdate,
		Time: &metav1.Time{Time: c.now},
	}}
	return edited
}

// buildPodRestartKeepsLinks deletes an API pod that calls its cache;
// its replacement comes up ready.
func buildPodRestartKeepsLinks(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	api := serviceCallApp(c)
	c.after(10 * time.Minute)
	c.remove(api.pod(1, "n1"))
	api.setReady(1)
	c.update(api.objects())
	c.after(20 * time.Second)
	c.create(api.pod(2, "n1", startedNow, notReady))
	c.after(15 * time.Second)
	c.update(api.pod(2, "n1", startedNow))
	api.setReady(2)
	c.update(api.objects())
	c.after(10 * time.Minute)
}
