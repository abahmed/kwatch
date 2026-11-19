package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gapClusterScenarios are failures of the network between pods, the
// control plane's store and the objects that route traffic into the
// cluster.
func gapClusterScenarios() []scenario {
	return []scenario{
		gapPacketLoss(), gapEtcdSpaceExceeded(), gapIngressClassUnknown(),
		gapGatewayClassMissing(), gapConversionWebhookGone(),
	}
}

// gapPacketLoss: the ledger database runs healthy on n3; its callers on
// n1 and n2 time out on connect again and again while no NetworkPolicy
// was touched. The evidence cannot separate the node's network from the
// database, but the database itself answers its probes: nothing in the
// cluster's own state (no node condition, no event, no policy) names
// the node, so no cause is stated.
func gapPacketLoss() scenario {
	return scenario{
		expect: expectation{
			Name: "network-packet-loss-to-node",
			Description: "Callers of a healthy database time out " +
				"connecting to it; its pods run on one node, its " +
				"Service lists them ready and no policy changed.",
			Root: "unknown", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{
				"networkpolicy/shop/default-deny", "node//n1", "node//n2"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			db := c.deployment("shop", "ledger",
				"registry.example.com/ledger:4", 2)
			c.list(db.objects())
			pods := []*corev1.Pod{db.pod(0, "n3"), db.pod(1, "n3")}
			c.list(pods[0], pods[1])
			// The address the callers dial is the Service's ClusterIP.
			svc := clusterService(c, "shop", "ledger", 5432)
			svc.Spec.ClusterIP = "10.96.14.7"
			svc.Spec.ClusterIPs = []string{"10.96.14.7"}
			c.list(svc, clusterSlice(c, "shop", "ledger", pods...))
			api := c.deployment("shop", "api",
				"registry.example.com/api:2.5", 2)
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(2 * time.Minute)
			message := "level=fatal msg=\"connect ledger\" err=\"dial " +
				"tcp 10.96.14.7:5432: i/o timeout\""
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(api.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), api.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				api.setReady(0)
				c.update(api.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gapEtcdSpaceExceeded: etcd hits its space quota and raises the NOSPACE
// alarm; reads work but every write is refused, so the cluster stops
// accepting changes. etcd is the root; the API server still answers.
func gapEtcdSpaceExceeded() scenario {
	return scenario{
		expect: expectation{
			Name: "etcd-database-space-exceeded",
			Description: "etcd's database exceeds its quota; the API " +
				"server serves reads but rejects every write with " +
				"\"mvcc: database space exceeded\".",
			Root: "etcd//etcd", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"apiserver//kube-apiserver"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			controlPlaneFleet(c)
			c.probe(kube.Etcd, "")
			c.probe(kube.APIServer, "")
			c.after(time.Minute)
			for range 5 {
				c.probe(kube.Etcd, "[-]etcd failed: etcdserver: mvcc: "+
					"database space exceeded")
				c.probe(kube.APIServer, "")
				c.after(time.Minute)
			}
		},
	}
}

// gapIngressClassUnknown: an Ingress names ingressClassName
// nginx-internal, but the only controller in the cluster serves class
// nginx; no controller picks it up and it never gets an address.
func gapIngressClassUnknown() scenario {
	return scenario{
		expect: expectation{
			Name: "ingress-class-unknown",
			Description: "An Ingress names an IngressClass that no " +
				"controller serves; it never gets an address while the " +
				"backend and the controller are healthy.",
			Root: "ingress/shop/shop", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"service/shop/web",
				"deployment/shop/web",
				"deployment/ingress-nginx/controller"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			ctrl := c.deployment("ingress-nginx", "controller",
				"registry.k8s.io/ingress-nginx/controller:v1.11.2", 1)
			c.list(ctrl.objects())
			c.list(ctrl.pod(0, "n1"))
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			class := "nginx-internal"
			ingress := trafficIngress(c, "web", "")
			ingress.Spec.IngressClassName = &class
			c.create(ingress)
			for range 4 {
				c.after(5 * time.Minute)
				c.update(ingress.DeepCopy())
			}
		},
	}
}

// gapGatewayClassMissing: a Gateway names GatewayClass envoy-internal,
// which does not exist, so no controller reconciles it; Accepted and
// Programmed stay Unknown with "Waiting for controller".
func gapGatewayClassMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "gateway-class-missing",
			Description: "A Gateway names a GatewayClass that does not " +
				"exist; its Accepted and Programmed conditions stay " +
				"Unknown, waiting for a controller.",
			Root: "gateway.gateway.networking.k8s.io/shop/public",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/web", "node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			gateway := trafficGateway(c)
			_ = unstructured.SetNestedField(gateway.Object,
				"envoy-internal", "spec", "gatewayClassName")
			at := c.now.UTC().Format(time.RFC3339)
			var conditions []any
			for _, kind := range []string{"Accepted", "Programmed"} {
				conditions = append(conditions, map[string]any{
					"type": kind, "status": "Unknown", "reason": "Pending",
					"message":            "Waiting for controller",
					"observedGeneration": int64(1),
					"lastTransitionTime": at,
				})
			}
			_ = unstructured.SetNestedSlice(gateway.Object, conditions,
				"status", "conditions")
			c.create(gateway)
			for range 4 {
				c.after(5 * time.Minute)
				c.update(gateway.DeepCopy())
			}
		},
	}
}

// gapConversionWebhookGone: a CustomResourceDefinition converts between
// its versions through a webhook Service whose Deployment was deleted;
// the operator that lists the resources fails on every read with "no
// endpoints available for service".
func gapConversionWebhookGone() scenario {
	root := "customresourcedefinition//widgets.acme.example"
	return scenario{
		expect: expectation{
			Name: "crd-conversion-webhook-gone",
			Description: "A CRD's conversion webhook has no backend; the " +
				"operator that reads its resources crash-loops on " +
				"\"no endpoints available for service\".",
			Root: root, Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/acme-system/widget-operator",
				"node//n1"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			crd := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata":   map[string]any{"name": "widgets.acme.example"},
				"spec": map[string]any{
					"group": "acme.example",
					"conversion": map[string]any{
						"strategy": "Webhook",
						"webhook": map[string]any{"clientConfig": map[string]any{
							"service": map[string]any{
								"namespace": c.n("acme-system"),
								"name":      c.n("widget-conversion"),
								"path":      "/convert",
							}}},
					},
				},
			}}
			crd.SetUID("crd-widgets")
			crd.SetCreationTimestamp(metav1.NewTime(
				c.start.Add(-24 * time.Hour)))
			c.list(crd)
			// The endpoint slice controller keeps an empty slice for a
			// Service whose pods are gone.
			c.list(clusterService(c, "acme-system", "widget-conversion",
				443), clusterSlice(c, "acme-system", "widget-conversion"))
			conversion := c.deployment("acme-system", "widget-conversion",
				"registry.example.com/widget-conversion:1", 0)
			c.list(conversion.objects())
			op := c.deployment("acme-system", "widget-operator",
				"registry.example.com/widget-operator:3", 1)
			c.list(op.objects())
			c.list(op.pod(0, "n1"))
			c.after(time.Minute)
			message := "conversion webhook for acme.example/v1alpha1, " +
				"Kind=Widget failed: Post \"https://widget-conversion." +
				"acme-system.svc:443/convert?timeout=30s\": no " +
				"endpoints available for service \"widget-conversion\""
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(op.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)))
				op.setReady(0)
				c.update(op.objects())
				c.after(time.Minute)
			}
		},
	}
}
