package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// readyZeroScenarios are workloads with no ready replica whose pods
// never restart, so only their readiness says what is wrong.
func readyZeroScenarios() []scenario {
	return []scenario{readyNeverService(), knownNeverReadyPastBoot()}
}

// readyNeverService: a one-replica Deployment starts and its pod fails
// readiness every ten seconds for 45 minutes, never restarting. The
// Service in front of it has no ready endpoint the whole time.
func readyNeverService() scenario {
	return scenario{
		expect: expectation{
			Name: "ready-never-service",
			Description: "A Deployment's only pod runs for 45 minutes " +
				"without restarting, its readiness probe answers 500 " +
				"every ten seconds and its Service has no endpoints.",
			Root: "deployment/external-secrets/cert-controller",
			Tier: "notify", MaxMessages: 3, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			w := certController(c)
			slice := admissionSlice(c, "external-secrets",
				"cert-controller", 8080, w.pod(0, "n1"))
			setEndpointsReady(slice, false)
			c.list(clusterService(c, "external-secrets",
				"cert-controller", 8080), slice)
			unreadyFor(c, w, 45*time.Minute)
		},
	}
}

// knownNeverReadyPastBoot: the same Deployment was not ready for twelve
// minutes at the start of each of two earlier days and was heard about
// both times, so it is known. On the third day its only pod is still not
// ready after 45 minutes. Past the boot window that is not routine boot
// noise, so the incident is announced again.
func knownNeverReadyPastBoot() scenario {
	return scenario{
		expect: expectation{
			Name: "known-never-ready-past-boot",
			Description: "A known not-ready start lasts 45 minutes with " +
				"no ready replica and no restart.",
			Root: "deployment/external-secrets/cert-controller",
			Tier: "notify", MaxMessages: 8, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			w := certController(c)
			for day, length := range []time.Duration{
				12 * time.Minute, 12 * time.Minute, 45 * time.Minute,
			} {
				began := c.now
				unreadyFor(c, w, length)
				c.update(w.pod(0, "n1", createdAt(began),
					readyAfter(c.now.Sub(began))))
				w.setReady(1)
				c.update(w.objects())
				c.after(23*time.Hour + time.Duration(day)*time.Minute)
			}
		},
	}
}

// certController lists one node and a one-replica Deployment whose
// readiness probe is /readyz, with its only pod ready.
func certController(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("external-secrets", "cert-controller",
		"registry.example.com/certs:1.0", 1)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/readyz", Port: intstr.FromInt32(8081)}},
			InitialDelaySeconds: 20, PeriodSeconds: 5, FailureThreshold: 3,
		}
	})
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	c.after(time.Minute)
	return w
}

// unreadyFor makes the only pod fail readiness with HTTP 500, as the
// kubelet reports it every ten seconds, for d. It never restarts.
func unreadyFor(c *cluster, w *workload, d time.Duration) {
	pod := w.pod(0, "n1", startedNow, notReady)
	w.setReady(0)
	c.update(w.objects())
	c.update(pod)
	probe := "Readiness probe failed: HTTP probe failed with " +
		"statuscode: 500"
	for count := int32(1); time.Duration(count)*10*time.Second <= d; count++ {
		c.after(10 * time.Second)
		ev := c.warningEvent(pod, "Pod", "Unhealthy", probe, "kubelet",
			count)
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
	}
}
