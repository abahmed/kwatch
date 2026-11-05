package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// wordingRuleScenarios lock what a message says when the evidence is
// specific: a health check that answers, a workload scaled to zero by a
// release, a custom resource's own error, and kwatch's own restart.
func wordingRuleScenarios() []scenario {
	return []scenario{
		probeAnswersOnUndeclaredPort(), scaledToZeroWithRelease(),
		customConditionDetail(), customGenericWithEvent(),
		ownVolumeWait("own-volume-wait-at-start", 4*time.Minute),
		ownVolumeWait("own-volume-wait-too-long", 15*time.Minute),
	}
}

// probeAnswersOnUndeclaredPort: a controller's readiness probe asks
// /readyz on port 8081, which its spec does not declare (it listens
// there through a flag), and the check answers HTTP 500. Something
// answers, so the port is not blamed: the check's answer is said.
func probeAnswersOnUndeclaredPort() scenario {
	return scenario{
		expect: expectation{
			Name: "readiness-answers-on-undeclared-port",
			Description: "A readiness probe on a port the container " +
				"does not declare answers HTTP 500: the answer is " +
				"said, the port is not blamed.",
			Root: "deployment/infra/cert-agent", Tier: "notify",
			MaxMessages: 2, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("infra", "cert-agent",
				"registry.example.com/cert-agent:0.9", 1)
			for _, spec := range []*corev1.PodSpec{
				&w.deployment.Spec.Template.Spec,
				&w.replicaSet.Spec.Template.Spec,
			} {
				app := &spec.Containers[0]
				app.Args = []string{"--healthz-addr=:8081"}
				app.Ports = []corev1.ContainerPort{{Name: "metrics",
					ContainerPort: 8080}}
				app.ReadinessProbe = &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/readyz",
							Port: intstr.FromInt(8081)}},
					PeriodSeconds: 5, FailureThreshold: 3,
				}
			}
			w.setReady(0)
			c.list(w.objects())
			pod := w.pod(0, "n1", notReady)
			c.list(pod)
			for n := int32(1); n <= 12; n++ {
				c.after(time.Minute)
				ev := c.warningEvent(pod, "Pod", "Unhealthy",
					"Readiness probe failed: HTTP probe failed with "+
						"statuscode: 500", "kubelet", n*12)
				ev.InvolvedObject.FieldPath = "spec.containers{app}"
				c.warn(ev)
			}
		},
	}
}

// scaledToZeroWithRelease: a deploy tool sets a new image and zero
// replicas in one apply while an Ingress still routes to the Service.
// The workload is scaled to zero, not failing after a release.
func scaledToZeroWithRelease() scenario {
	return scenario{
		expect: expectation{
			Name: "scaled-to-zero-with-release",
			Description: "One apply changes the image and scales a " +
				"Deployment to zero while an Ingress still routes to " +
				"it: it is scaled to zero, not a failing release.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api",
				"registry.example.com/api:1.0", 2)
			c.list(w.objects())
			c.list(clusterService(c, "shop", "api", 8080))
			c.list(trafficIngress(c, "api", ""))
			c.after(30 * time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/api:1.1"
			})
			zero := int32(0)
			w.deployment.Spec.Replicas = &zero
			w.setReady(0)
			w.deployment.ManagedFields = []metav1.ManagedFieldsEntry{{
				Manager:   "deploy-tool",
				Operation: metav1.ManagedFieldsOperationApply,
				Time:      &metav1.Time{Time: c.now},
			}}
			c.update(w.deployment)
			c.create(rs)
		},
	}
}

// certIssuer is a namespaced certificate issuer of a certificate
// controller, which reports its state as a Ready condition.
func certIssuer(c *cluster, name string) *unstructured.Unstructured {
	meta := clusterMeta(c, "certs", name, "issuer")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "certs.example.com/v1", "kind": "Issuer",
		"spec":   map[string]any{},
		"status": map[string]any{"observedGeneration": int64(1)},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	return u
}

// customConditionDetail: an issuer is not ready; the condition message
// ends in the detail after " :: ". The title carries that detail, not
// the reason's keyword.
func customConditionDetail() scenario {
	return scenario{
		expect: expectation{
			Name: "custom-condition-detail",
			Description: "A custom resource's Ready=False message ends " +
				"in a specific detail: the title says it.",
			Root: "issuer.certs.example.com/certs/public", Tier: "notify",
			MaxMessages: 2, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			issuer := certIssuer(c, "public")
			clusterCondition(issuer, "Ready", "False",
				"ErrRegisterAccount", "Failed to register account: 400 "+
					"urn:example:error:invalidContact: Error validating "+
					"contact(s) :: contact email has forbidden domain "+
					"\"example.com\"", c.now)
			c.list(issuer)
			c.after(10 * time.Minute)
		},
	}
}

// customGenericWithEvent: a synced secret object is not ready with a
// generic message; its controller's Warning event says why. The title
// carries the event's words.
func customGenericWithEvent() scenario {
	return scenario{
		expect: expectation{
			Name: "custom-generic-with-event",
			Description: "A custom resource's Ready=False message is " +
				"generic and its newest Warning event says why: the " +
				"title says the event's words.",
			Root: "issuer.certs.example.com/certs/internal",
			Tier: "notify", MaxMessages: 2,
			Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			issuer := certIssuer(c, "internal")
			clusterCondition(issuer, "Ready", "False", "SyncError",
				"could not get secret data from provider", c.now)
			c.list(issuer)
			for n := int32(1); n <= 6; n++ {
				c.after(2 * time.Minute)
				c.warn(c.warningEvent(issuer, "Issuer", "UpdateFailed",
					"Secret does not exist", "certs-controller", n))
			}
		},
	}
}

// ownVolumeWait: kwatch itself (in its own namespace) restarts, and its
// new pod waits for its ReadWriteOnce volume to leave the old node for
// wait. A few minutes is every restart and stays quiet; a wait past the
// start window is news.
func ownVolumeWait(name string, wait time.Duration) scenario {
	e := expectation{
		Name: name,
		Description: "kwatch's own new pod waits for its volume after a " +
			"restart: quiet within the start window, told after it.",
		Tail: duration(5 * time.Minute),
	}
	if wait < 10*time.Minute {
		e.Quiet = true
	} else {
		e.Root, e.Tier, e.MaxMessages = "deployment/kwatch/kwatch",
			"notify", 2
		// Held on purpose by kwatch's own start window.
		e.BootHeld = true
	}
	return scenario{expect: e, build: func(c *cluster) {
		buildOwnVolumeWait(c, wait)
	}}
}

func buildOwnVolumeWait(c *cluster, wait time.Duration) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	claim := storageBoundClaim(c, "kwatch", "data", "fast", "2Gi",
		"pv-kwatch")
	c.list(scheduleClass(c, "fast", storagev1.VolumeBindingImmediate),
		storageVolume(c, claim, "pv-kwatch"), claim)
	w := c.deployment("kwatch", "kwatch",
		"registry.example.com/kwatch:1.2", 1)
	storageMount(w, claim.Name)
	w.setReady(0)
	c.list(w.objects())
	fresh := w.pod(0, "n2", waiting("ContainerCreating", ""))
	c.list(fresh)
	waits := int32(wait / (30 * time.Second))
	for n := int32(1); n <= waits; n++ {
		c.after(30 * time.Second)
		c.warn(c.warningEvent(fresh, "Pod", "FailedAttachVolume",
			"Multi-Attach error for volume \"pv-kwatch\" Volume is "+
				"already exclusively attached to one node and can't be "+
				"attached to another", "attachdetach-controller", n))
	}
	w.setReady(1)
	c.update(w.objects())
	c.update(w.pod(0, "n2", readyJustNow))
}
