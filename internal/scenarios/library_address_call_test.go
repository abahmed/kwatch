package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// addressCallScenarios are failures whose error names an address, not
// a Service name.
func addressCallScenarios() []scenario {
	return []scenario{policyBlocksCallByAddress()}
}

// policyBlocksCallByAddress: an invoicer's ConfigMap is edited (a
// setting it ignores) and, seconds later, an egress policy that only
// allows DNS is created. The pods time out dialing 10.96.31.8:5432,
// the virtual address of the ledger Service, which their environment
// does not name. The address belongs to the Service, so the policy
// that blocks it is the root, not the ConfigMap.
func policyBlocksCallByAddress() scenario {
	return scenario{
		expect: expectation{
			Name: "policy-blocks-call-by-address",
			Description: "A ConfigMap edit and an egress NetworkPolicy " +
				"land together; the pods time out dialing the address " +
				"of a Service their environment does not name.",
			Root: "networkpolicy/shop/dns-only-egress", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n2", "configmap/shop/invoicer-settings"},
			Tail:         duration(10 * time.Minute),
		},
		build: buildPolicyByAddress,
	}
}

func buildPolicyByAddress(c *cluster) {
	c.list(c.node("n2", "zone-a"), c.node("n3", "zone-a"))
	ledger := c.deployment("shop", "ledger",
		"registry.example.com/ledger:11", 1)
	c.list(ledger.objects())
	ledgerPod := ledger.pod(0, "n3")
	service := clusterService(c, "shop", "ledger", 5432)
	service.Spec.ClusterIP = "10.96.31.8"
	// The API server defaults the target port to the port.
	service.Spec.Ports[0].TargetPort = intstr.FromInt32(5432)
	c.list(ledgerPod, service, trafficSlice(c, "ledger", ledgerPod))
	cm := configMap(c, "shop", "invoicer-settings",
		map[string]string{"locale": "en"})
	app := c.deployment("shop", "invoicer",
		"registry.example.com/invoicer:8", 2)
	configTemplate(app, func(spec *corev1.PodSpec) {
		spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cm.Name}}}}
	})
	c.list(cm)
	c.list(app.objects())
	c.list(app.pod(0, "n2"), app.pod(1, "n2"))
	c.after(time.Minute)
	edited := last(c, cm)
	edited.Data["locale"] = "de"
	c.update(edited)
	c.after(10 * time.Second)
	c.create(dnsOnlyEgress(c, app))
	c.after(20 * time.Second)
	message := "FATAL: could not connect to server: dial tcp " +
		"10.96.31.8:5432: i/o timeout"
	for restarts := int32(1); restarts <= 5; restarts++ {
		for replica := range 2 {
			c.update(app.pod(replica, "n2", crashLoop(1, "Error", message,
				restarts)))
		}
		app.setReady(0)
		c.update(app.objects())
		c.after(45 * time.Second)
	}
}

// dnsOnlyEgress selects the workload's pods and allows only DNS out.
func dnsOnlyEgress(c *cluster, w *workload) *networkingv1.NetworkPolicy {
	udp, tcp := corev1.ProtocolUDP, corev1.ProtocolTCP
	dns := intstr.FromInt32(53)
	return &networkingv1.NetworkPolicy{
		ObjectMeta: clusterMeta(c, "shop", "dns-only-egress", "netpol"),
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: w.deployment.Spec.Template.Labels},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &udp, Port: &dns},
					{Protocol: &tcp, Port: &dns},
				},
			}},
		},
	}
}
