package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// finalizerScenarios are deletions that hold for good: objects waiting
// for a finalizer that nothing runs, and attachments to volumes that are
// gone.
func finalizerScenarios() []scenario {
	return []scenario{
		finalizerControllerScaledDown(), finalizerControllerRunning(),
		finalizerControllerScaledDownFresh(), attachmentVolumeGone(),
		attachmentNodeGone(),
	}
}

const (
	// leftoverAge is how long the leftovers have been stuck: weeks.
	leftoverAge = 10 * 24 * time.Hour
	// controllerGroup is the API group of the leftovers' controller.
	controllerGroup = "widgets.example.com"
	// widgetFinalizer is the finalizer the leftovers wait for.
	widgetFinalizer = controllerGroup + "/cleanup"
)

// finalizerControllerScaledDown: six objects of one namespace were
// deleted ten days ago; the controller that removes their finalizers is
// scaled to zero. They are one leftover, named by that controller, and
// they wait for the digest.
func finalizerControllerScaledDown() scenario {
	return scenario{
		expect: expectation{
			Name: "finalizer-controller-scaled-down",
			Description: "Six objects have been stuck deleting for ten " +
				"days; the Deployment that handles their finalizers " +
				"is scaled to zero.",
			Root: "deployment/widget-system/widget-controller",
			Tier: "digest", MaxMessages: 2, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(finalizerController(c, 0).objects())
			c.list(leftovers(c, leftoverAge)...)
		},
	}
}

// finalizerControllerRunning: the same leftovers, but their controller
// runs. Nothing says it is down, so it is not blamed; the leftovers are
// still one incident, named by the finalizer.
func finalizerControllerRunning() scenario {
	return scenario{
		expect: expectation{
			Name: "finalizer-controller-running",
			Description: "Six objects have been stuck deleting for ten " +
				"days; the Deployment that handles their finalizers " +
				"runs.",
			Root: "finalizer/widget-system/" + widgetFinalizer,
			Tier: "digest", MaxMessages: 2, Tail: duration(30 * time.Minute),
			MustNotBlame: []string{"deployment/widget-system/" +
				"widget-controller"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := finalizerController(c, 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.list(leftovers(c, leftoverAge)...)
		},
	}
}

// finalizerControllerScaledDownFresh: the same objects were deleted two
// hours ago. Someone is probably waiting for them, so it is news.
func finalizerControllerScaledDownFresh() scenario {
	return scenario{
		expect: expectation{
			Name: "finalizer-controller-scaled-down-fresh",
			Description: "Six objects have been stuck deleting for two " +
				"hours; the Deployment that handles their finalizers " +
				"is scaled to zero.",
			Root: "deployment/widget-system/widget-controller",
			Tier: "notify", MaxMessages: 2, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(finalizerController(c, 0).objects())
			c.list(leftovers(c, 2*time.Hour)...)
		},
	}
}

// attachmentVolumeGone: two attachments were deleted a year ago and the
// volumes they attach no longer exist. Each is named by its volume, not
// by its hash, and waits for the digest.
func attachmentVolumeGone() scenario {
	return scenario{
		expect: expectation{
			Name: "attachment-volume-gone",
			Description: "Two VolumeAttachments are stuck deleting; " +
				"the volumes they attach no longer exist.",
			Root:       "volumeattachment//csi-0a1b2c",
			OtherRoots: []string{"volumeattachment//csi-3d4e5f"},
			Tier:       "digest", MaxMessages: 2,
			Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(staleAttachment(c, "csi-0a1b2c", "pvc-1111", "n1"),
				staleAttachment(c, "csi-3d4e5f", "pvc-2222", "n1"))
		},
	}
}

// attachmentNodeGone: an attachment was deleted a year ago; its volume
// exists but its node was removed, so the volume cannot attach to
// another node. That is still news.
func attachmentNodeGone() scenario {
	return scenario{
		expect: expectation{
			Name: "attachment-node-gone",
			Description: "A VolumeAttachment is stuck deleting; its " +
				"node no longer exists but its volume does.",
			Root: "volumeattachment//csi-6a7b8c", Tier: "notify",
			MaxMessages: 2, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(&corev1.PersistentVolume{
				ObjectMeta: c.meta("", "pvc-3333")})
			c.list(staleAttachment(c, "csi-6a7b8c", "pvc-3333",
				"removed-node"))
		},
	}
}

// staleAttachment is a VolumeAttachment of a node that was deleted a
// year ago, for a volume.
func staleAttachment(
	c *cluster, name, volume, node string,
) *storagev1.VolumeAttachment {
	meta := c.meta("", name)
	stamp := metav1.NewTime(c.now.Add(-365 * 24 * time.Hour))
	meta.DeletionTimestamp = &stamp
	meta.Finalizers = []string{"external-attacher/ebs-csi-example-com"}
	return &storagev1.VolumeAttachment{ObjectMeta: meta,
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: "ebs.csi.example.com", NodeName: c.n(node),
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &volume}}}
}

// finalizerController is the Deployment that runs the controller of the
// widgets API group; its pods carry the group in their labels.
func finalizerController(c *cluster, replicas int32) *workload {
	w := c.deployment("widget-system", "widget-controller",
		"registry.example.com/widget-controller:1.2", replicas)
	w.deployment.Spec.Template.Labels[controllerGroup+
		"/controller-namespace"] = c.n("widget-system")
	return w
}

// leftovers are six objects of widget-system that were deleted age ago
// and still wait for a finalizer: a Widget and its set, and four
// objects the Widget owns.
func leftovers(c *cluster, age time.Duration) []runtime.Object {
	const rbac = "rbac.authorization.k8s.io/v1"
	api := controllerGroup + "/v1"
	owner := leftover(c, api, "Widget", "widget-a", age, nil)
	return []runtime.Object{owner,
		leftover(c, api, "WidgetSet", "set-a", age, nil),
		leftover(c, "v1", "Secret", "widget-a-secret", age, owner),
		leftover(c, "v1", "Secret", "widget-a-token", age, owner),
		leftover(c, rbac, "Role", "widget-a-role", age, owner),
		leftover(c, rbac, "RoleBinding", "widget-a-binding", age, owner),
	}
}

// leftover builds an object of widget-system deleted age ago and held
// by the widget finalizer, owned by owner when it has one.
func leftover(
	c *cluster, apiVersion, kind, name string, age time.Duration,
	owner *unstructured.Unstructured,
) *unstructured.Unstructured {
	meta := clusterMeta(c, "widget-system", name, kind)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion, "kind": kind}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	stamp := metav1.NewTime(c.now.Add(-age))
	u.SetDeletionTimestamp(&stamp)
	u.SetFinalizers([]string{widgetFinalizer})
	if owner != nil {
		u.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: owner.GetAPIVersion(), Kind: owner.GetKind(),
			Name: owner.GetName(), UID: owner.GetUID(),
		}})
	}
	return u
}
