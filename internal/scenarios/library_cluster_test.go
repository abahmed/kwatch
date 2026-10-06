package scenarios

import (
	"fmt"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// clusterScenarios are failures of shared cluster services: DNS,
// admission, quotas, registries, metrics APIs, network policy and
// operators.
func clusterScenarios() []scenario {
	return []scenario{
		clusterDNSDown(), clusterWebhookNoEndpoints(),
		clusterWebhookSharedBackend(),
		clusterQuotaExhausted(), clusterMetricsAPIDown(),
		clusterRegistryAuth(), clusterImageTypo(),
		clusterNetworkPolicyChange(), clusterOperatorCRStuck(),
	}
}

// clusterDNSDown: CoreDNS crash-loops, kwatch's own lookup fails and apps
// in two namespaces crash on name resolution. Cluster DNS is the root.
func clusterDNSDown() scenario {
	return scenario{
		expect: expectation{
			Name: "coredns-down",
			Description: "CoreDNS pods crash-loop; kwatch's DNS probe " +
				"fails and apps in several namespaces crash with " +
				"name-resolution errors.",
			Root: "cluster-dns//cluster-dns", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2",
				"deployment/shop/cart", "deployment/billing/invoices"},
		},
		build: buildClusterDNSDown,
	}
}

func buildClusterDNSDown(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-b"))
	dns := c.deployment("kube-system", "coredns",
		"registry.k8s.io/coredns/coredns:v1.11.1", 2)
	cart := c.deployment("shop", "cart", "registry.example.com/cart:3.1", 2)
	invoices := c.deployment("billing", "invoices",
		"registry.example.com/invoices:1.8", 2)
	for _, w := range []*workload{dns, cart, invoices} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.probe(kube.ClusterDNS, "")
	c.after(time.Minute)
	loop := "[FATAL] plugin/loop: Loop (127.0.0.1:53 -> :53) detected " +
		"for zone \".\""
	for restarts := int32(1); restarts <= 6; restarts++ {
		c.update(dns.pod(0, "n1", crashLoop(1, "Error", loop, restarts)),
			dns.pod(1, "n2", crashLoop(1, "Error", loop, restarts)))
		dns.setReady(0)
		c.update(dns.objects())
		c.probe(kube.ClusterDNS, "lookup kubernetes.default.svc."+
			"cluster.local on 10.96.0.10:53: read udp 10.244.1.7:41234"+
			"->10.96.0.10:53: i/o timeout")
		if restarts >= 2 {
			clusterDNSVictims(c, cart, "db.shop.svc.cluster.local",
				restarts-1)
			clusterDNSVictims(c, invoices,
				"payments-api.billing.svc.cluster.local", restarts-1)
		}
		c.after(30 * time.Second)
	}
}

// clusterDNSVictims crash both replicas of w on a lookup of host.
func clusterDNSVictims(c *cluster, w *workload, host string, n int32) {
	message := "Error: dial tcp: lookup " + c.n(host) + " on " +
		"10.96.0.10:53: no such host"
	if n%2 == 0 {
		message = "getaddrinfo EAI_AGAIN " + c.n(host) +
			": Temporary failure in name resolution"
	}
	c.update(w.pod(0, "n1", crashLoop(1, "Error", message, n)),
		w.pod(1, "n2", crashLoop(1, "Error", message, n)))
	w.setReady(0)
	c.update(w.objects())
}

// clusterWebhookNoEndpoints: the policy webhook's Deployment is scaled to
// zero while its configuration still fails closed, so no controller can
// create pods. The blocking webhook configuration is the root.
func clusterWebhookNoEndpoints() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-no-endpoints",
			Description: "A fail-closed validating webhook loses every " +
				"endpoint; ReplicaSets in two namespaces cannot create " +
				"pods.",
			Root:        "validatingwebhookconfiguration//policy-validator",
			Tier:        "page",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/orders",
				"deployment/search/indexer"},
		},
		build: buildClusterWebhook,
	}
}

// clusterWebhookSharedBackend: three webhook configurations call one
// Service whose Deployment is scaled to zero, and two more call a Service
// that does not exist. Each backend is one root; its webhooks are its
// symptoms, not five incidents.
func clusterWebhookSharedBackend() scenario {
	return scenario{
		expect: expectation{
			Name: "webhook-shared-backend",
			Description: "Three fail-closed webhooks call a Service " +
				"with no ready endpoints and two call a Service that " +
				"does not exist; each backend is one incident.",
			Root:        "service/policy/policy-svc",
			OtherRoots:  []string{"service/mesh-system/mesh-webhook"},
			Tier:        "page",
			MaxMessages: 2,
			MustNotBlame: []string{
				"validatingwebhookconfiguration//mesh-validator",
				"validatingwebhookconfiguration//mesh-default-validator",
				"mutatingwebhookconfiguration//mesh-injector",
				"validatingwebhookconfiguration//policy-cleanup",
				"validatingwebhookconfiguration//policy-exceptions",
			},
		},
		build: buildClusterWebhookSharedBackend,
	}
}

func buildClusterWebhookSharedBackend(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	mesh := c.deployment("mesh-system", "mesh-webhook",
		"registry.example.com/mesh:1.2", 2)
	c.list(mesh.objects())
	pods := []*corev1.Pod{mesh.pod(0, "n1"), mesh.pod(1, "n2")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "mesh-system", "mesh-webhook", 443))
	c.list(clusterSlice(c, "mesh-system", "mesh-webhook", pods...))
	c.list(clusterValidatingHook(c, "mesh-validator",
		"validate.mesh.example.com", "mesh-system", "mesh-webhook"))
	c.list(clusterValidatingHook(c, "mesh-default-validator",
		"default.mesh.example.com", "mesh-system", "mesh-webhook"))
	c.list(admissionMutatingHook(c, "mesh-injector",
		"inject.mesh.example.com", "mesh-system", "mesh-webhook", 10))
	c.list(clusterValidatingHook(c, "policy-cleanup",
		"cleanup.policy.example.com", "policy", "policy-svc"))
	c.list(clusterValidatingHook(c, "policy-exceptions",
		"exceptions.policy.example.com", "policy", "policy-svc"))
	c.after(time.Minute)
	setReplicas(mesh, 0)
	mesh.setReady(0)
	c.update(mesh.objects())
	c.remove(pods[0], pods[1])
	c.update(clusterSlice(c, "mesh-system", "mesh-webhook"))
	c.after(3 * time.Minute)
}

func buildClusterWebhook(c *cluster) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	hook := c.deployment("policy-system", "policy-webhook",
		"registry.example.com/policy-webhook:0.9", 2)
	c.list(hook.objects())
	pods := []*corev1.Pod{hook.pod(0, "n1"), hook.pod(1, "n2")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "policy-system", "policy-webhook", 8443))
	c.list(clusterSlice(c, "policy-system", "policy-webhook", pods...))
	c.list(clusterValidatingHook(c, "policy-validator",
		"validate.policy.example.com", "policy-system", "policy-webhook"))
	orders := c.deployment("shop", "orders", "registry.example.com/o:2", 2)
	indexer := c.deployment("search", "indexer",
		"registry.example.com/indexer:5", 2)
	for _, w := range []*workload{orders, indexer} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(time.Minute)
	setReplicas(hook, 0)
	hook.setReady(0)
	c.update(hook.objects())
	c.remove(pods[0], pods[1])
	c.update(clusterSlice(c, "policy-system", "policy-webhook"))
	c.after(30 * time.Second)
	message := "Internal error occurred: failed calling webhook " +
		"\"validate.policy.example.com\": failed to call webhook: Post " +
		"\"https://" + c.n("policy-webhook") + "." + c.n("policy-system") +
		".svc:8443/validate?timeout=10s\": no endpoints available for " +
		"service \"" + c.n("policy-webhook") + "\""
	for i, w := range []*workload{orders, indexer} {
		setReplicas(w, 4)
		w.setReady(2)
		c.update(w.objects())
		for n := int32(1); n <= 4; n++ {
			c.warn(c.warningEvent(w.replicaSet, "ReplicaSet",
				"FailedCreate", "Error creating: "+message,
				"replicaset-controller", 3*n+int32(i)))
			c.after(45 * time.Second)
		}
	}
}

// clusterQuotaExhausted: a scale-up hits the namespace's compute quota;
// the ReplicaSet cannot create the new pods. The quota is the root.
func clusterQuotaExhausted() scenario {
	return scenario{
		expect: expectation{
			Name: "quota-exhausted",
			Description: "A Deployment scales from 4 to 8 replicas in a " +
				"namespace whose ResourceQuota is used up; the new pods " +
				"are forbidden.",
			Root: "resourcequota/analytics/compute-quota", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: buildClusterQuota,
	}
}

func buildClusterQuota(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("analytics", "ingest",
		"registry.example.com/ingest:4.0", 4)
	c.list(w.objects())
	for i := range 4 {
		c.list(w.pod(i, "n1"))
	}
	quota := clusterQuota(c, "3", "4")
	c.list(quota)
	c.after(time.Minute)
	setReplicas(w, 8)
	w.setReady(4)
	c.update(w.objects())
	c.update(clusterQuota(c, "4", "4"))
	for n := int32(1); n <= 5; n++ {
		message := fmt.Sprintf("Error creating: pods \"%s-%c\" is "+
			"forbidden: exceeded quota: %s, requested: requests.cpu="+
			"1, used: requests.cpu=4, limited: requests.cpu=4",
			w.replicaSet.Name, 'e'+rune(n), c.n("compute-quota"))
		c.warn(c.warningEvent(w.replicaSet, "ReplicaSet", "FailedCreate",
			message, "replicaset-controller", n*4))
		c.after(time.Minute)
	}
}

func clusterQuota(c *cluster, used, hard string) *corev1.ResourceQuota {
	limits := corev1.ResourceList{
		corev1.ResourceRequestsCPU: resource.MustParse(hard),
	}
	return &corev1.ResourceQuota{
		ObjectMeta: clusterMeta(c, "analytics", "compute-quota", "quota"),
		Spec:       corev1.ResourceQuotaSpec{Hard: limits},
		Status: corev1.ResourceQuotaStatus{
			Hard: limits,
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse(used),
			},
		},
	}
}

// clusterMetricsAPIDown: metrics-server was uninstalled, so the resource
// metrics APIService has no endpoints and every HPA stops scaling. The
// APIService is the root.
func clusterMetricsAPIDown() scenario {
	return scenario{
		expect: expectation{
			Name: "metrics-apiservice-down",
			Description: "APIService v1beta1.metrics.k8s.io loses its " +
				"backend; three HPAs cannot read CPU metrics.",
			Root: "apiservice//v1beta1.metrics.k8s.io", Tier: "notify",
			MaxMessages: 2,
		},
		build: buildClusterMetricsAPI,
	}
}

func buildClusterMetricsAPI(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	c.list(clusterService(c, "kube-system", "metrics-server", 10250))
	c.list(clusterAPIService(c, "True", "Passed", "all checks passed"))
	namespaces := []string{"shop", "billing", "search"}
	for _, ns := range namespaces {
		w := c.deployment(ns, "web", "registry.example.com/web:7", 2)
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n1"))
		c.list(clusterHPA(c, ns, "web", "True", "ValidMetricFound", ""))
	}
	c.after(time.Minute)
	c.update(clusterSlice(c, "kube-system", "metrics-server"))
	c.update(clusterAPIService(c, "False", "MissingEndpoints",
		"endpoints for service/"+c.n("metrics-server")+" in \""+
			c.n("kube-system")+"\" have no addresses with port name "+
			"\"https\""))
	message := "the HPA was unable to compute the replica count: failed " +
		"to get cpu utilization: unable to get metrics for resource " +
		"cpu: unable to fetch metrics from resource metrics API: the " +
		"server is currently unable to handle the request (get " +
		"pods.metrics.k8s.io)"
	c.after(30 * time.Second)
	// The failure lasts past HPAMetricsGrace. The HPA condition changes
	// once; later rounds only repeat the controller's event.
	for n := int32(1); n <= 24; n++ {
		for _, ns := range namespaces {
			hpa := clusterHPA(c, ns, "web", "False",
				"FailedGetResourceMetric", message)
			if n == 1 {
				c.update(hpa)
			}
			c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
				"FailedGetResourceMetric", message,
				"horizontal-pod-autoscaler", n))
		}
		c.after(30 * time.Second)
	}
}

func clusterHPA(
	c *cluster, namespace, name, active, reason, message string,
) *autoscalingv2.HorizontalPodAutoscaler {
	meta := clusterMeta(c, namespace, name, "hpa")
	minReplicas := int32(2)
	since := metav1.NewTime(c.now)
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: meta,
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: meta.Name,
			},
			MinReplicas: &minReplicas, MaxReplicas: 10,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 2, DesiredReplicas: 2,
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{Type: autoscalingv2.AbleToScale, Status: "True",
					Reason: "ReadyForNewScale", LastTransitionTime: since},
				{Type: autoscalingv2.ScalingActive,
					Status: corev1.ConditionStatus(active), Reason: reason,
					Message: message, LastTransitionTime: since},
			},
		},
	}
}

func clusterAPIService(
	c *cluster, status, reason, message string,
) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{
			"name": "v1beta1.metrics.k8s.io", "uid": "apiservice-metrics",
		},
		"spec": map[string]any{
			"group": "metrics.k8s.io", "version": "v1beta1",
			"service": map[string]any{
				"namespace": c.n("kube-system"),
				"name":      c.n("metrics-server"),
			},
		},
		"status": map[string]any{"conditions": []any{map[string]any{
			"type": "Available", "status": status, "reason": reason,
			"message":            message,
			"lastTransitionTime": c.now.UTC().Format(time.RFC3339),
		}}},
	}}
	return u
}

func clusterService(
	c *cluster, namespace, name string, port int32,
) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: clusterMeta(c, namespace, name, "svc"),
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": c.n(name)},
			Ports:    []corev1.ServicePort{{Port: port}},
		},
	}
}

// clusterSlice is the Service's EndpointSlice with one ready endpoint
// per pod; no pods means no endpoints.
func clusterSlice(
	c *cluster, namespace, service string, pods ...*corev1.Pod,
) *discoveryv1.EndpointSlice {
	meta := clusterMeta(c, namespace, service+"-x1", "slice")
	meta.Labels = map[string]string{discoveryv1.LabelServiceName: c.n(service)}
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: meta, AddressType: discoveryv1.AddressTypeIPv4,
	}
	for i, pod := range pods {
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{
			Addresses:  []string{fmt.Sprintf("10.244.0.%d", 10+i)},
			Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)},
			TargetRef: &corev1.ObjectReference{
				Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name,
			},
		})
	}
	return slice
}

func clusterValidatingHook(
	c *cluster, name, webhook, namespace, service string,
) *admissionv1.ValidatingWebhookConfiguration {
	fail := admissionv1.Fail
	none := admissionv1.SideEffectClassNone
	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: clusterMeta(c, "", name, "hook"),
		Webhooks: []admissionv1.ValidatingWebhook{{
			Name: webhook, FailurePolicy: &fail, SideEffects: &none,
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Namespace: c.n(namespace), Name: c.n(service),
				},
			},
		}},
	}
}

// clusterMeta is c.meta with a UID distinct per kind, so a Service and
// the Deployment of the same name never share one.
func clusterMeta(c *cluster, namespace, name, kind string) metav1.ObjectMeta {
	meta := c.meta(namespace, name)
	meta.UID += "-" + types.UID(kind)
	return meta
}
