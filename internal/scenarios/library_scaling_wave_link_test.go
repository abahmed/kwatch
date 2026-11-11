package scenarios

import "time"

// wavePullDenied is the kubelet's text when the registry refuses the
// pull.
const wavePullDenied = "Back-off pulling image " +
	"\"registry.example.com/web:4\": ErrImagePull: failed to pull " +
	"and unpack image: failed to resolve reference: pulling from " +
	"host registry.example.com failed with status code [manifests " +
	"4]: 401 Unauthorized"

// scaleDownUnrelatedOutageTold: during the same scale-down, the only
// pod of another routed Deployment stops being ready. It has run for an
// hour on n5, a node the scaling never touched: the scaling does not
// explain it, so it is told as on any other day, without waiting for
// the nodes, while web's moved pod stays quiet.
func scaleDownUnrelatedOutageTold() scenario {
	return scenario{
		expect: expectation{
			Name: "scale-down-unrelated-outage-told",
			Description: "Three of five nodes are removed within two " +
				"minutes; meanwhile the only pod of another routed " +
				"Deployment, on a node that stays, stops being ready.",
			Root: "deployment/shop/pay", Tier: "notify", MaxMessages: 2,
			Alone: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			pay := c.deployment("shop", "pay",
				"registry.example.com/pay:9", 1)
			c.list(pay.objects())
			pod := pay.pod(0, "n5")
			c.list(pod, clusterService(c, "shop", "pay", 8080),
				trafficSlice(c, "pay", pod), trafficIngress(c, "pay", ""))
			web, nodes := waveFleet(c)
			born := waveScaleDown(c, web, nodes)
			c.update(pay.pod(0, "n5", notReady))
			pay.setReady(0)
			c.update(pay.objects())
			c.update(trafficSlice(c, "pay"))
			c.after(4 * time.Minute)
			waveStartsOn(c, web, born)
		},
	}
}

// scaleDownMovedPodPullFails: the moved pod finds room on n5 a minute
// later, but the registry refuses its pull. A pull that failed is an
// error, not a wait for the nodes: it pages without waiting for them.
func scaleDownMovedPodPullFails() scenario {
	return scenario{
		expect: expectation{
			Name: "scale-down-moved-pod-pull-fails",
			Description: "Three of five nodes are removed within two " +
				"minutes; the moved pod of a routed Deployment is " +
				"scheduled on a remaining node and its image pull is " +
				"refused.",
			Root: "deployment/shop/web", Tier: "page", MaxMessages: 2,
			Alone: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			web, nodes := waveFleet(c)
			born := waveScaleDown(c, web, nodes)
			c.after(time.Minute)
			c.update(movedPod(web, "n5", born,
				waiting("ImagePullBackOff", wavePullDenied)))
			c.after(10 * time.Minute)
		},
	}
}
