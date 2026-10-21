package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// rcaEvidenceProbeScenarios are crashes next to a probe aimed at a port
// the container does not declare.
func rcaEvidenceProbeScenarios() []scenario {
	return []scenario{undeclaredProbePortCodeCrash()}
}

// undeclaredProbePortCodeCrash: the container declares 8080 and probes
// the admin port 8081, which is valid and long-standing; the app then
// crashes on a database error. Only a probe that cannot connect says
// nothing listens: a crash with another message is not the probe's.
func undeclaredProbePortCodeCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "undeclared-probe-port-code-crash",
			Description: "A readiness probe targets an undeclared admin " +
				"port that works; the app crashes on a database error.",
			Root: "deployment/shop/orders", Tier: "notify",
			MaxMessages: 4,
		},
		build: func(c *cluster) {
			w := c.deployment("shop", "orders",
				"registry.example.com/orders:4.1", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				ctr := &spec.Containers[0]
				ctr.Ports = []corev1.ContainerPort{{ContainerPort: 8080}}
				ctr.ReadinessProbe = &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/ready",
							Port: intstr.FromInt32(8081)}}}
			})
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(30 * time.Minute)
			for round := int32(1); round <= 4; round++ {
				for i := 0; i < 2; i++ {
					c.update(w.pod(i, "n"+itoa(i+1), crashLoop(1, "Error",
						"fatal: password authentication failed for "+
							"user orders", round*3)))
				}
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}
