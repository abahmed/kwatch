package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gapSpecScenarios are Deployment edits that name something that is not
// there, or that never take effect.
func gapSpecScenarios() []scenario {
	return []scenario{
		gapPriorityClassMissing(), gapDeploymentPaused(),
		gapConfigMapVolumeMissing(), gapNodeNameMissing(),
	}
}

// gapPriorityClassMissing: a release names PriorityClass batch-critical,
// which does not exist; the API server refuses every pod the new
// ReplicaSet tries to create, while the old pods keep serving.
func gapPriorityClassMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "release-priority-class-missing",
			Description: "A release names a PriorityClass that does not " +
				"exist; the ReplicaSet reports FailedCreate \"no " +
				"PriorityClass with name batch-critical was found\".",
			Root: "priorityclass//batch-critical", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1",
				"deployment/shop/api"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.PriorityClassName = "batch-critical"
			})
			message := "Error creating: pods \"" + w.replicaSet.Name +
				"-\" is forbidden: no PriorityClass with name " +
				"batch-critical was found"
			condition := appsv1.DeploymentCondition{
				Type: appsv1.DeploymentReplicaFailure, Status: "True",
				Reason: "FailedCreate", Message: message,
				LastTransitionTime: metav1.NewTime(c.now),
			}
			d := w.deployment.DeepCopy()
			d.Status.UpdatedReplicas = 0
			d.Status.Replicas, d.Status.UnavailableReplicas = 2, 0
			d.Status.Conditions = []appsv1.DeploymentCondition{condition}
			c.update(d)
			for m := int32(1); m <= 8; m++ {
				c.warn(c.warningEvent(w.replicaSet, "ReplicaSet",
					"FailedCreate", message, "replicaset-controller", m))
				c.after(time.Minute)
			}
		},
	}
}

// gapDeploymentPaused: someone paused a Deployment and then edited its
// image; nothing rolls out, and for an hour the pods run the old image.
func gapDeploymentPaused() scenario {
	return scenario{
		expect: expectation{
			Name: "deployment-paused-with-pending-edit",
			Description: "A Deployment is paused and then edited; the " +
				"change never rolls out and the Progressing condition " +
				"reads DeploymentPaused.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:2.5",
				2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(5 * time.Minute)
			d := w.deployment.DeepCopy()
			d.Spec.Paused = true
			d.Spec.Template.Spec.Containers[0].Image =
				"registry.example.com/api:2.6"
			d.Generation, d.Status.ObservedGeneration = 2, 2
			d.Status.Conditions = []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentProgressing, Status: "Unknown",
				Reason: "DeploymentPaused", Message: "Deployment is paused",
				LastTransitionTime: metav1.NewTime(c.now),
			}}
			c.update(d)
			for range 4 {
				c.after(15 * time.Minute)
				c.update(d.DeepCopy())
			}
		},
	}
}

// gapConfigMapVolumeMissing: a release mounts the ConfigMap app-config
// as a volume but nobody created it; the new pod waits in
// ContainerCreating with FailedMount.
func gapConfigMapVolumeMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "release-configmap-volume-missing",
			Description: "A release mounts a ConfigMap volume that does " +
				"not exist; the new pod stays in ContainerCreating with " +
				"FailedMount \"configmap not found\".",
			Root: "configmap/shop/app-config", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1",
				"deployment/shop/api"},
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{{Name: "app-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.
								LocalObjectReference{Name: c.n("app-config")},
						},
					}}}
				spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
					Name: "app-config", MountPath: "/etc/api",
				}}
			})
			created := c.now
			pod := w.pod(0, "n1", createdAt(created),
				gapPending(waiting("ContainerCreating", "")))
			c.create(pod)
			for round := int32(1); round <= 6; round++ {
				c.after(time.Minute)
				gapKubelet(c, pod, "FailedMount", "MountVolume.SetUp "+
					"failed for volume \"app-config\" : configmap \""+
					c.n("app-config")+"\" not found", round)
			}
			gapKubelet(c, pod, "FailedMount", "Unable to attach or "+
				"mount volumes: unmounted volumes=[app-config], "+
				"unattached volumes=[app-config kube-api-access]: "+
				"timed out waiting for the condition", 1)
		},
	}
}

// gapNodeNameMissing: a release pins spec.nodeName to a node that was
// removed with its pool. The scheduler never sees the pod and the kubelet
// that would run it does not exist, so the pod is Pending without a
// single event.
func gapNodeNameMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "release-node-name-missing",
			Description: "A release sets spec.nodeName to a node that " +
				"does not exist; the new pod is Pending with no " +
				"scheduler or kubelet events.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//gpu-worker-07", "node//n1"},
			Tail:         duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			w := gapRelease(c, "api", func(spec *corev1.PodSpec) {
				spec.NodeName = c.n("gpu-worker-07")
			})
			pinned := func(c *cluster, pod *corev1.Pod) {
				pod.Spec.NodeName = c.n("gpu-worker-07")
				pod.CreationTimestamp = metav1.NewTime(c.now)
				pod.Status = corev1.PodStatus{
					Phase: corev1.PodPending, QOSClass: corev1.PodQOSBurstable,
				}
			}
			pod := w.pod(0, "", pinned)
			c.create(pod)
			for range 3 {
				c.after(5 * time.Minute)
				c.update(pod.DeepCopy())
			}
		},
	}
}
