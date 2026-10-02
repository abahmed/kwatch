package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// controlPlaneScenarios are failures of the control plane that show up
// across the whole cluster: no pod gets scheduled, no controller acts.
func controlPlaneScenarios() []scenario {
	return []scenario{schedulerDown(), controllerManagerDown(), etcdDown()}
}

// schedulerDown: the scheduler's leader lease stops renewing. Three
// workloads scale out and every new pod stays Pending without a node and
// without a scheduling condition: nobody looked at them. The scheduler is
// the root, not the workloads or the nodes.
func schedulerDown() scenario {
	return scenario{
		expect: expectation{
			Name: "scheduler-down",
			Description: "The scheduler stops renewing its lease; new " +
				"pods of three workloads stay Pending with no node and " +
				"no scheduling condition.",
			Root: "scheduler//kube-scheduler", Tier: "page",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/cart",
				"deployment/shop/search", "deployment/billing/invoices",
				"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := controlPlaneFleet(c)
			c.probe(kube.Scheduler, "")
			c.after(time.Minute)
			c.probe(kube.Scheduler, "leader lease not renewed for 1m35s")
			c.after(30 * time.Second)
			for _, w := range fleet {
				setReplicas(w, 3)
				w.setReady(2)
				c.update(w.objects())
				c.create(w.pod(2, "", notScheduled))
			}
			for range 4 {
				c.after(time.Minute)
				c.probe(kube.Scheduler, "leader lease not renewed for "+
					"3m5s")
			}
		},
	}
}

// controllerManagerDown: the controller manager's lease stops renewing.
// Three Deployments are edited and none of them is ever observed: their
// observed generation stays behind. The controller manager is the root.
func controllerManagerDown() scenario {
	return scenario{
		expect: expectation{
			Name: "controller-manager-down",
			Description: "The controller manager stops renewing its " +
				"lease; three edited Deployments are never reconciled.",
			Root: "controller-manager//kube-controller-manager",
			Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/cart",
				"deployment/shop/search", "deployment/billing/invoices"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := controlPlaneFleet(c)
			c.probe(kube.ControllerManager, "")
			c.after(time.Minute)
			c.probe(kube.ControllerManager,
				"leader lease not renewed for 1m40s")
			c.after(30 * time.Second)
			for _, w := range fleet {
				edited := w.deployment.DeepCopy()
				edited.Generation++
				edited.Spec.Template.Annotations = map[string]string{
					"kubectl.kubernetes.io/restartedAt": c.now.UTC().
						Format(time.RFC3339),
				}
				w.deployment = edited
				c.update(edited)
			}
			for range 5 {
				c.after(time.Minute)
				c.probe(kube.ControllerManager,
					"leader lease not renewed for 3m10s")
			}
		},
	}
}

// etcdDown: etcd loses quorum; the API server's health check reports
// etcd failing and the API server stops answering. etcd is the root,
// not the API server that depends on it.
func etcdDown() scenario {
	return scenario{
		expect: expectation{
			Name: "etcd-down",
			Description: "etcd fails its health check and the API " +
				"server behind it stops answering.",
			Root: "etcd//etcd", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"apiserver//kube-apiserver"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			controlPlaneFleet(c)
			c.probe(kube.Etcd, "")
			c.probe(kube.APIServer, "")
			c.after(time.Minute)
			c.probe(kube.Etcd, "[-]etcd failed: error getting data "+
				"from etcd: context deadline exceeded")
			c.after(20 * time.Second)
			c.probe(kube.APIServer, "Get \"https://10.96.0.1:443/"+
				"readyz\": context deadline exceeded")
			for range 4 {
				c.after(time.Minute)
				c.probe(kube.Etcd, "[-]etcd failed: error getting data "+
					"from etcd: context deadline exceeded")
				c.probe(kube.APIServer, "Get \"https://10.96.0.1:443/"+
					"readyz\": context deadline exceeded")
			}
		},
	}
}

// controlPlaneFleet lists three healthy two-replica Deployments.
func controlPlaneFleet(c *cluster) []*workload {
	fleet := []*workload{
		c.deployment("shop", "cart", "registry.example.com/cart:3.1", 2),
		c.deployment("shop", "search", "registry.example.com/search:2", 2),
		c.deployment("billing", "invoices",
			"registry.example.com/invoices:1.8", 2),
	}
	for _, w := range fleet {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	return fleet
}

// notScheduled is a new pod no scheduler has looked at: no node, no
// scheduling condition, no start time.
func notScheduled(c *cluster, pod *corev1.Pod) {
	pod.Spec.NodeName = ""
	pod.CreationTimestamp = metav1.NewTime(c.now)
	pod.Status = corev1.PodStatus{
		Phase: corev1.PodPending, QOSClass: corev1.PodQOSBurstable,
	}
}
