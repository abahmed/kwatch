package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// probeAnswerScenarios are readiness probes on a port the container
// does not declare whose target answers: nothing is wrong with the port.
func probeAnswerScenarios() []scenario {
	return []scenario{probeRefusedThenHTTP500()}
}

// probeRefusedThenHTTP500: cert-controller declares only port 8080 and
// probes /readyz on 8081. While it starts, the probe finds nothing
// listening; then the app listens on 8081 and answers HTTP 500 for
// twenty minutes. The port is right: the readiness check fails.
func probeRefusedThenHTTP500() scenario {
	return scenario{
		expect: expectation{
			Name: "probe-refused-then-http-500",
			Description: "A readiness probe on an undeclared port is " +
				"refused while the app starts, then answers HTTP 500 " +
				"for twenty minutes.",
			Root: "deployment/external-secrets/cert-controller",
			Tier: "notify", MaxMessages: 3,
			Tail: duration(5 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("external-secrets", "cert-controller",
				"registry.example.com/certs:1.0", 1)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Containers[0].Ports = []corev1.ContainerPort{
					{ContainerPort: 8080}}
				spec.Containers[0].ReadinessProbe = &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/readyz",
							Port: intstr.FromInt32(8081)}},
					InitialDelaySeconds: 20, PeriodSeconds: 5,
					FailureThreshold: 3,
				}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(time.Minute)
			pod := w.pod(0, "n1", startedNow, notReady)
			w.setReady(0)
			c.update(w.objects())
			c.update(pod)
			refused := "Readiness probe failed: Get \"http://10.0.1.4:" +
				"8081/readyz\": dial tcp 10.0.1.4:8081: connect: " +
				"connection refused"
			answered := "Readiness probe failed: HTTP probe failed " +
				"with statuscode: 500"
			for count := int32(1); count <= 126; count++ {
				c.after(10 * time.Second)
				text := answered
				if count <= 6 {
					text = refused
				}
				ev := c.warningEvent(pod, "Pod", "Unhealthy", text,
					"kubelet", count)
				ev.InvolvedObject.FieldPath = "spec.containers{app}"
				c.warn(ev)
			}
		},
	}
}
