package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// csiRegistrationScenarios are pods whose volume cannot be mounted
// because the storage driver is not registered on their node.
func csiRegistrationScenarios() []scenario {
	return []scenario{csiNodePluginFailing()}
}

// csiDriver is the name of the storage driver of one cluster. It carries
// the cluster's suffix, so two scenarios of one staging day do not share
// a driver (an error naming it would then fit both of their plugins).
func csiDriver(c *cluster) string { return c.n("ebs.csi.example.com") }

// csiNotRegistered is the kubelet's mount error for a driver that is not
// registered on the node.
func csiNotRegistered(c *cluster) string {
	return "MountVolume.MountDevice failed for volume " +
		"\"pvc-1a2b\" : kubernetes.io/csi: attacher.MountDevice failed " +
		"to create newCsiDriverClient: driver name " + csiDriver(c) +
		" not found in the list of registered CSI drivers"
}

// csiNodePlugin is the CSI node plugin DaemonSet, a pod on each of two
// nodes, and returns it. Its container names the driver it registers.
func csiNodePlugin(c *cluster) *appsv1.DaemonSet {
	labels := map[string]string{"app": "ebs-csi-node"}
	ds := &appsv1.DaemonSet{
		ObjectMeta: c.meta("kube-system", "ebs-csi-node"),
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels,
				"registry.example.com/aws-ebs-csi-driver:v1.30"),
		},
	}
	ds.Spec.Template.Spec.Containers[0].Args = []string{
		"node", "--driver-name=" + csiDriver(c)}
	ds.Generation, ds.Status.ObservedGeneration = 1, 1
	setAgentCounts(ds, 2, 2)
	return ds
}

// csiNodePluginFailing: the node plugin on n1 crash-loops, so the
// driver is not registered there, and the api pod on n1 cannot mount
// its volume. The plugin is the root, not the pod or the node.
func csiNodePluginFailing() scenario {
	return scenario{
		expect: expectation{
			Name: "csi-node-plugin-failing",
			Description: "A pod cannot mount its volume because the " +
				"CSI driver is not registered on its node, whose node " +
				"plugin pod crash-loops.",
			Root: "daemonset/kube-system/ebs-csi-node", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n1"},
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			ds := csiNodePlugin(c)
			c.list(ds, agentPod(c, ds, "n1"), agentPod(c, ds, "n2"))
			api := c.deployment("shop", "api",
				"registry.example.com/api:4", 1)
			c.list(api.objects())
			c.after(10 * time.Minute)
			setAgentCounts(ds, 2, 1)
			c.update(ds)
			for round := int32(1); round <= 4; round++ {
				c.update(agentPod(c, ds, "n1", crashLoop(1, "Error",
					"failed to dial the kubelet registration socket",
					round*3)))
				pod := api.pod(0, "n1", waiting("ContainerCreating", ""))
				pod.CreationTimestamp = metav1.NewTime(c.now)
				pod.Status.Phase = corev1.PodPending
				if round == 1 {
					c.create(pod)
				} else {
					c.update(pod)
				}
				c.warn(c.warningEvent(pod, "Pod", "FailedMount",
					csiNotRegistered(c), "kubelet", round*3))
				c.after(time.Minute)
			}
			c.after(5 * time.Minute)
		},
	}
}
