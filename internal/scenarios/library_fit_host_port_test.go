package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// hostPort makes the first container claim a node port.
func hostPort(port int32) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) {
		spec.Containers[0].Ports = []corev1.ContainerPort{{
			ContainerPort: 8080, HostPort: port,
			Protocol: corev1.ProtocolTCP}}
	}
}

// fitHostPortTaken: a Deployment whose pods each claim host port 8080
// has one replica on each of the two nodes; the third cannot start
// because both nodes already hold the port. CPU is plentiful, so the
// port is the blocker; the message names the pod holding it on each node.
func fitHostPortTaken() scenario {
	return scenario{
		expect: expectation{
			Name: "fit-host-port-taken",
			Description: "A third replica asking for host port 8080 is " +
				"Pending: both nodes already run a replica that holds " +
				"the port.",
			Root: "scheduling//didn't have free ports for the requested " +
				"pod ports",
			Tier:        "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			c.list(fitPoolNode(c, "n1", "zone-a", "general", "8"),
				fitPoolNode(c, "n2", "zone-a", "general", "8"))
			w := c.deployment("edge", "proxy",
				"registry.example.com/proxy:1.4", 3)
			w.rollout(func(spec *corev1.PodSpec) {
				requestCPU("250m")(spec)
				hostPort(8080)(spec)
			})
			w.setReady(2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			message := "0/2 nodes are available: 2 node(s) didn't have " +
				"free ports for the requested pod ports. preemption: " +
				"0/2 nodes are available: 2 No preemption victims " +
				"found for incoming pod."
			c.create(w.pod(2, "", pendingUnscheduled(message)))
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				pod := last(c, w.pod(2, ""))
				c.warn(c.warningEvent(pod, "Pod", "FailedScheduling",
					message, "default-scheduler", m))
			}
		},
	}
}
