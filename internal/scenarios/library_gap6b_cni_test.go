package scenarios

import (
	"time"
)

// gap6bCNIDeleteFails: the CNI plugin of n2 cannot tear down sandboxes
// because its state is gone; deleted pods stay Terminating.
func gap6bCNIDeleteFails() scenario {
	return scenario{
		expect: g2Exp("node-cni-teardown-fails-pods-terminating",
			"After a CNI upgrade deleted pods on one node stay "+
				"Terminating with FailedKillPod \"failed to destroy "+
				"network for sandbox\"; rollouts stall.",
			"node//n2", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, []string{"n1", "n2"})
			c.after(10 * time.Minute)
			message := "failed to \"KillPodSandbox\" for \"6a1c\" with " +
				"KillPodSandboxError: \"rpc error: code = Unknown desc " +
				"= failed to destroy network for sandbox: plugin " +
				"type=\\\"calico\\\" failed (delete): error getting " +
				"ClusterInformation: connection is unauthorized\""
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n2", notReady)
					deleting(&pod.ObjectMeta, c.now.Add(-time.Minute))
					c.update(pod)
					gapKubelet(c, pod, "FailedKillPod", message, round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap6bCNIConfigInvalid: a CNI add-on upgrade left an invalid network
// configuration on n3; new pods there fail sandbox creation while the
// running ones are fine.
func gap6bCNIConfigInvalid() scenario {
	return scenario{
		expect: g2Exp("node-cni-config-invalid-new-pods-fail",
			"New pods on one node fail sandbox creation because the "+
				"node's CNI configuration is invalid (code 7); pods "+
				"on other nodes start.",
			"node//n3", "notify", "deployment/shop/api").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			gap6bContainerCreating(c, w, 2, "n3", "FailedCreatePodSandBox",
				"Failed to create pod sandbox: rpc error: code = Unknown "+
					"desc = failed to setup network for sandbox: plugin "+
					"type=\"bridge\" failed (add): invalid network config "+
					"(code 7): invalid CNI configuration: ipam: missing "+
					"field \"subnet\"", 6)
		},
	}
}

// gap6bCNIPluginMissing: the new node image lacks the portmap plugin; every
// new pod on n4, from three namespaces, fails sandbox creation.
func gap6bCNIPluginMissing() scenario {
	return scenario{
		expect: g2Exp("node-image-lacks-cni-plugin-binary",
			"Pods of three namespaces placed on a node with a new image "+
				"fail sandbox creation: the portmap plugin binary is "+
				"missing; other nodes are fine.",
			"node//n4", "notify", "deployment/shop/web",
			"deployment/billing/invoices", "deployment/media/transcode").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n4", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, []string{"n1"})
			c.after(10 * time.Minute)
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
			}
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n4", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedCreatePodSandBox", "Failed "+
						"to create pod sandbox: rpc error: code = Unknown "+
						"desc = failed to setup network for sandbox: "+
						"plugin type=\"portmap\" failed (add): failed to "+
						"find plugin \"portmap\" in path [/opt/cni/bin]",
						round)
				}
				c.after(time.Minute)
			}
		},
	}
}
