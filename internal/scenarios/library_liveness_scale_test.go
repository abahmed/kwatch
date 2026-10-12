package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// livenessScaleOutTooTight: the transcoder scales out. The two running
// replicas needed about a minute to become ready, but the liveness
// probe gives up after 18 seconds, so every added replica is killed
// (exit 143) while it starts, again and again. The probe event names no
// container. The Deployment's probe is the root.
func livenessScaleOutTooTight() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-scale-out-too-tight",
			Description: "Added replicas of a slow-starting app are " +
				"killed by a liveness probe that gives up after 18 " +
				"seconds while running replicas needed a minute.",
			Root: "deployment/media/transcoder", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//w1", "node//w2", "node//w3"},
		},
		build: buildLivenessScaleOut,
	}
}

func buildLivenessScaleOut(c *cluster) {
	nodes := []string{"w1", "w2", "w3"}
	for _, node := range nodes {
		c.list(c.node(node, "zone-b"))
	}
	w := c.deployment("media", "transcoder",
		"registry.example.com/transcoder:3.1", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Ports = []corev1.ContainerPort{{
			Name: "http", ContainerPort: 9000}}
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/alive", Port: intstr.FromInt32(9000)}},
			InitialDelaySeconds: 8, PeriodSeconds: 5, FailureThreshold: 2,
		}
	})
	c.list(w.objects())
	c.list(w.pod(0, "w1", readyAfter(62*time.Second)),
		w.pod(1, "w2", readyAfter(66*time.Second)))
	c.after(4 * time.Minute)
	setReplicas(w, 5)
	w.setReady(2)
	c.update(w.objects())
	killed := "Liveness probe failed: Get \"http://10.244.3.8:9000/alive\":" +
		" dial tcp 10.244.3.8:9000: connect: connection refused"
	for restarts := int32(1); restarts <= 5; restarts++ {
		for i := 2; i < 5; i++ {
			pod := w.pod(i, nodes[i%3], startedNow,
				crashLoop(143, "Error", "", restarts))
			pod.CreationTimestamp = metav1.NewTime(c.now.Add(
				-time.Duration(restarts) * 40 * time.Second))
			c.update(pod)
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", killed,
				"kubelet", restarts*2))
		}
		c.after(40 * time.Second)
	}
}
