package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap3EphemeralQuota: a pod with a generic ephemeral volume waits for the
// claim the ephemeral volume controller must create; the namespace's
// storage quota refuses it.
func gap3EphemeralQuota() scenario {
	return scenario{
		expect: g2Exp("ephemeral-volume-claim-refused-by-quota",
			"A pod with a generic ephemeral volume stays Pending "+
				"waiting for its claim, which the namespace's storage "+
				"quota refuses.",
			"resourcequota/shop/storage-quota", "notify", "node//n1",
			"deployment/shop/api").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			storage := corev1.ResourceList{
				corev1.ResourceRequestsStorage: resource.MustParse("100Gi")}
			c.list(gap3Quota(c, "shop", "storage-quota", storage, storage))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 1)
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: waiting for ephemeral " +
				"volume controller to create the persistentvolumeclaim " +
				"\"" + w.pod(0, "").Name + "-scratch\". preemption: 0/1 " +
				"nodes are available: 1 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 8)
		},
	}
}

// gap3ReplicaSetQuota: the object-count quota for ReplicaSets is full, so
// a rollout cannot even create its new ReplicaSet; the old pods serve.
func gap3ReplicaSetQuota() scenario {
	return scenario{
		expect: g2Exp("deployment-replicaset-create-refused-by-quota",
			"A rollout cannot create its new ReplicaSet because the "+
				"namespace's object-count quota is full; the old pods "+
				"keep serving.",
			"resourcequota/shop/object-counts", "notify", "node//n1",
			"deployment/shop/api").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:2", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			count := corev1.ResourceList{"count/replicasets.apps": resource.
				MustParse("10")}
			c.list(gap3Quota(c, "shop", "object-counts", count, count))
			c.after(10 * time.Minute)
			message := "Failed to create new replica set \"api-5d9c\": " +
				"replicasets.apps \"api-5d9c\" is forbidden: exceeded " +
				"quota: object-counts, requested: count/replicasets." +
				"apps=1, used: count/replicasets.apps=10, limited: " +
				"count/replicasets.apps=10"
			d := w.deployment.DeepCopy()
			d.Spec.Template.Spec.Containers[0].Image =
				"registry.example.com/api:3"
			d.Generation++
			d.Status.Conditions = []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentProgressing, Status: "False",
				Reason: "ReplicaSetCreateError", Message: message,
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(d)
			for m := int32(1); m <= 6; m++ {
				g2Event(c, d, "Deployment", "ReplicaSetCreateError",
					message, "deployment-controller", m)
				c.after(time.Minute)
			}
		},
	}
}
