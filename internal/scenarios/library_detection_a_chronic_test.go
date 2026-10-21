package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// chronicScenarios are failures older than kwatch: a unit that was
// already failing when kwatch started is still a current failure.
func chronicScenarios() []scenario {
	return []scenario{chronicNeverReadyDeployment()}
}

// chronicNeverReadyDeployment: kwatch starts while a controller's only
// pod has run for hours without passing its readiness probe and its
// Deployment has been unavailable for 26 days. The kubelet records the
// failed probe on the container, aggregated into one event whose count
// grows. The Deployment is reported, not left to history.
func chronicNeverReadyDeployment() scenario {
	return scenario{
		expect: expectation{
			Name: "chronic-never-ready-deployment",
			Description: "kwatch starts while a Deployment has been " +
				"unavailable for 26 days: its pod runs for hours " +
				"and never passes its readiness probe.",
			Root: "deployment/shop/cert-controller", Tier: "notify",
			MaxMessages: 3, Tail: duration(30 * time.Minute),
		},
		build: buildChronicNeverReady,
	}
}

func buildChronicNeverReady(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "cert-controller",
		"registry.example.com/cert-controller:1.0", 1)
	w.setReady(0)
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/readyz", Port: intstr.FromInt(8081)}},
		InitialDelaySeconds: 20, PeriodSeconds: 5, FailureThreshold: 3,
	}
	w.deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	w.replicaSet.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	w.deployment.Status.Conditions = []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse,
		Reason:             "MinimumReplicasUnavailable",
		LastTransitionTime: metav1.NewTime(c.now.Add(-26 * 24 * time.Hour)),
	}}
	c.list(w.objects())
	pod := w.pod(0, "n1", createdAt(c.now.Add(-4*time.Hour)),
		notReadySince(c.now.Add(-4*time.Hour)))
	c.list(pod)
	for count := int32(2844); count <= 2880; count += 12 {
		c.after(5 * time.Minute)
		event := c.warningEvent(pod, "Pod", "Unhealthy",
			"Readiness probe failed: HTTP probe failed with "+
				"statuscode: 500", "kubelet", count)
		event.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(event)
	}
}
