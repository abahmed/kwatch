package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap2CtrlScenarios are storage binding, scheduling limits, scaling that
// oscillates and a StatefulSet whose Service is missing.
func gap2CtrlScenarios() []scenario {
	return []scenario{
		gap2ClaimAccessMode(), gap2MaxVolumeCount(), gap2HPAFlapping(),
		gap2HPASingleScale(), gap2ReplicasFight(), gap2StatefulSetService(),
	}
}

// gap2ClaimAccessMode: a claim asks for ReadWriteMany from a class whose
// driver supports only ReadWriteOnce; the provisioner refuses and the pod
// that mounts it waits.
func gap2ClaimAccessMode() scenario {
	return scenario{
		expect: g2Exp("pvc-rwx-class-supports-only-rwo",
			"A claim asks for ReadWriteMany from a StorageClass whose "+
				"driver supports only ReadWriteOnce; provisioning fails "+
				"and the pod that mounts it stays Pending.",
			"persistentvolumeclaim/shop/shared-uploads", "notify",
			"node//n1", "deployment/shop/uploads").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			claim := scheduleClaim(c, "shop", "shared-uploads", "gp3")
			claim.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany}
			c.list(scheduleClass(c, "gp3", "Immediate"))
			c.create(claim)
			w := c.deployment("shop", "uploads",
				"registry.example.com/uploads:3", 2)
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: pod has unbound " +
				"immediate PersistentVolumeClaims. preemption: 0/1 " +
				"nodes are available: 1 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)),
				w.pod(1, "", pendingUnscheduled(message)))
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				g2Event(c, claim, "PersistentVolumeClaim",
					"ProvisioningFailed", "failed to provision volume "+
						"with StorageClass \"gp3\": rpc error: code = "+
						"InvalidArgument desc = Volume capabilities "+
						"MULTI_NODE_MULTI_WRITER not supported. Only "+
						"AccessModes[ReadWriteOnce] supported.",
					"ebs.csi.aws.com_ebs-csi-controller", m)
			}
		},
	}
}

// gap2MaxVolumeCount: every node already has as many volumes attached as
// the driver allows; a new Deployment whose pods each need a volume stays
// Pending on "exceed max volume count".
func gap2MaxVolumeCount() scenario {
	return scenario{
		expect: g2Exp("node-max-volume-count-reached",
			"Every node already has its maximum number of volumes "+
				"attached; a new Deployment's pods are Pending with "+
				"\"exceed max volume count\" on all nodes.",
			"scheduling//exceed max volume count", "notify",
			"deployment/shop/web", "scheduling//Insufficient cpu",
			"scheduling//Too many pods").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			c.after(5 * time.Minute)
			w := c.deployment("shop", "web", "registry.example.com/web:7", 3)
			storageMount(w, c.n("web-data"))
			w.setReady(0)
			c.create(w.objects())
			message := "0/3 nodes are available: 3 node(s) exceed max " +
				"volume count. preemption: 0/3 nodes are available: 3 " +
				"No preemption victims found for incoming pod."
			for i := range 3 {
				c.create(w.pod(i, "", pendingUnscheduled(message)))
			}
			pendingForMinutes(c, w, message, 6)
		},
	}
}

// gap2Flap plays cycles of an HPA that scales web from 3 to 6 and back,
// each step a minute apart.
func gap2Flap(c *cluster, cycles int) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:7", 3)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"), w.pod(2, "n1"))
	hpa := clusterHPA(c, "shop", "web", "True", "ValidMetricFound", "")
	c.list(hpa)
	for range cycles {
		c.after(time.Minute)
		*w.deployment.Spec.Replicas = 6
		w.setReady(3)
		c.update(w.objects())
		for i := 3; i < 6; i++ {
			c.create(w.pod(i, "n1", startedNow, notReady))
		}
		h := last(c, hpa)
		h.Status.CurrentReplicas, h.Status.DesiredReplicas = 3, 6
		c.update(h)
		c.after(40 * time.Second)
		for i := 3; i < 6; i++ {
			c.update(w.pod(i, "n1", startedNow))
		}
		w.setReady(6)
		c.update(w.objects())
		h = last(c, hpa)
		h.Status.CurrentReplicas, h.Status.DesiredReplicas = 6, 6
		c.update(h)
		if cycles == 1 {
			continue
		}
		c.after(time.Minute)
		for i := 3; i < 6; i++ {
			c.remove(w.pod(i, "n1"))
		}
		*w.deployment.Spec.Replicas = 3
		w.setReady(3)
		c.update(w.objects())
		h = last(c, hpa)
		h.Status.CurrentReplicas, h.Status.DesiredReplicas = 3, 3
		c.update(h)
	}
	c.after(10 * time.Minute)
}

// gap2HPAFlapping: an oscillating metric makes the HPA scale the web
// Deployment from 3 to 6 and back every few minutes for half an hour;
// each scale-up starts three pods that are not ready yet.
func gap2HPAFlapping() scenario {
	return scenario{
		expect: g2Exp("hpa-flapping-scale-up-down",
			"An HPA scales a Deployment from three to six replicas and "+
				"back every few minutes for half an hour.",
			"horizontalpodautoscaler/shop/web", "digest", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) { gap2Flap(c, 8) },
	}
}

// gap2HPASingleScale: the same HPA scales the Deployment up once for a
// traffic peak and it stays there.
func gap2HPASingleScale() scenario {
	return scenario{
		expect: g2Quiet("hpa-single-scale-up-quiet",
			"An HPA scales a Deployment from three to six replicas once "+
				"and the new pods are ready within a minute."),
		build: func(c *cluster) { gap2Flap(c, 1) },
	}
}

// gap2ReplicasFight: an HPA raises the replicas of web to 6 and a GitOps
// controller applies the manifest's replicas: 3 again every few
// minutes; both appear as managers of the field.
func gap2ReplicasFight() scenario {
	return scenario{
		expect: g2Exp("deployment-replicas-reset-by-gitops",
			"An HPA and a GitOps controller both set the replicas of a "+
				"Deployment; the count flips between 3 and 6 every few "+
				"minutes.",
			"deployment/shop/web", "digest", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:7", 3)
			at := metav1.NewTime(c.start.Add(-time.Hour))
			field := []byte(`{"f:spec":{"f:replicas":{}}}`)
			w.deployment.ManagedFields = []metav1.ManagedFieldsEntry{
				{Manager: "argocd-application-controller",
					Operation:  metav1.ManagedFieldsOperationUpdate,
					FieldsType: "FieldsV1", Time: &at,
					FieldsV1: &metav1.FieldsV1{Raw: field}},
				{Manager: "kube-controller-manager",
					Operation:   metav1.ManagedFieldsOperationUpdate,
					Subresource: "scale", FieldsType: "FieldsV1", Time: &at,
					FieldsV1: &metav1.FieldsV1{Raw: field}},
			}
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"), w.pod(2, "n1"))
			for range 8 {
				c.after(3 * time.Minute)
				*w.deployment.Spec.Replicas = 6
				w.deployment.Generation++
				w.deployment.Status.ObservedGeneration++
				w.setReady(3)
				c.update(w.objects())
				c.after(time.Minute)
				*w.deployment.Spec.Replicas = 3
				w.deployment.Generation++
				w.deployment.Status.ObservedGeneration++
				w.setReady(3)
				c.update(w.objects())
			}
		},
	}
}

// gap2StatefulSetService: a StatefulSet names the governing Service
// queue, which does not exist; the brokers find no peers through the
// stable names and the ones that need them exit.
func gap2StatefulSetService() scenario {
	return scenario{
		expect: g2Exp("statefulset-governing-service-missing",
			"A StatefulSet's serviceName points to a Service that does "+
				"not exist; its pods fail to resolve their peers' stable "+
				"names and restart.",
			"service/streaming/queue", "notify", "node//n1",
			"node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			c.after(2 * time.Minute)
			message := "peer discovery failed: lookup queue-0.queue." +
				"streaming.svc.cluster.local on 10.96.0.10:53: no such host"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(operatorBroker(c, sts, 1, "n2", crashLoop(1,
					"Error", message, restarts)),
					operatorBroker(c, sts, 2, "n1", crashLoop(1, "Error",
						message, restarts)))
				c.after(time.Minute)
			}
		},
	}
}
