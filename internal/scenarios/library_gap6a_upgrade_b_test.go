package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap6aOwnerRefNamespace: after the upgrade the garbage collector treats
// an owner reference to another namespace as dangling and deletes the
// Deployment an operator created with it.
func gap6aOwnerRefNamespace() scenario {
	return scenario{
		expect: g2Exp("upgrade-garbage-collects-cross-namespace-owner-ref",
			"After an upgrade the garbage collector reports "+
				"OwnerRefInvalidNamespace for a Deployment whose owner "+
				"is in another namespace and deletes it and its pods.",
			"deployment/ops/report-worker", "notify", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("ops", "report-worker",
				"registry.example.com/report-worker:4", 2)
			yes := true
			w.deployment.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "reports.example.com/v1", Kind: "Report",
				Name: "weekly", UID: "uid-report-weekly", Controller: &yes}}
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(10 * time.Minute)
			g2Event(c, w.deployment, "Deployment", "OwnerRefInvalidNamespace",
				"ownerRef [reports.example.com/v1/Report, namespace: ops, "+
					"name: weekly, uid: uid-report-weekly] does not exist "+
					"in namespace \"ops\"", "garbage-collector-controller",
				1)
			c.after(30 * time.Second)
			c.remove(w.pod(0, "n1"), w.pod(1, "n1"))
			c.remove(w.objects())
			c.after(10 * time.Minute)
		},
	}
}

// gap6aRegistryRepoMoved: the upgrade points the add-ons at registry.k8s.io;
// the air-gapped mirror does not have it, so the next CoreDNS pod cannot
// pull its image.
func gap6aRegistryRepoMoved() scenario {
	e := g2Exp("upgrade-addon-image-repository-not-mirrored",
		"An upgrade moves the CoreDNS add-on to registry.k8s.io, which "+
			"the cluster's mirror lacks; its new pod stays in "+
			"ImagePullBackOff while the old one serves.",
		"deployment/kube-system/coredns", "notify", "node//n1",
		"node//n2").tail(10 * time.Minute)
	e.Ambiguous = true
	return scenario{expect: e, build: func(c *cluster) {
		c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
		w := c.deployment("kube-system", "coredns",
			"registry.example.com/mirror/coredns:v1.11.1", 2)
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
		c.after(5 * time.Minute)
		rs := w.rollout(func(spec *corev1.PodSpec) {
			spec.Containers[0].Image = "registry.k8s.io/coredns/coredns:v1.11.3"
		})
		d := w.deployment
		d.Status.Replicas, d.Status.UpdatedReplicas = 3, 1
		d.Status.ReadyReplicas, d.Status.AvailableReplicas = 2, 2
		d.Status.UnavailableReplicas = 1
		c.update(d)
		c.create(rs)
		g2Pull("Failed to pull image \"registry.k8s.io/coredns/coredns:"+
			"v1.11.3\": failed to resolve reference: failed to do "+
			"request: Head \"https://registry.k8s.io/v2/coredns/coredns/"+
			"manifests/v1.11.3\": dial tcp 34.96.108.209:443: i/o "+
			"timeout")(c, w)
	}}
}
