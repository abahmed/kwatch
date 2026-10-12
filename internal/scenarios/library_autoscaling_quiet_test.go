package scenarios

import "time"

// hpaAtMaxHealthy: the same autoscaler as hpa-at-max reaches its
// maximum of 6 replicas and reports ScalingLimited, but the replicas keep
// up and stay ready. A ceiling is a cause only once something fails on
// it, so nothing is sent.
func hpaAtMaxHealthy() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-at-max-healthy-quiet",
			Description: "An HPA sits at maxReplicas and reports " +
				"ScalingLimited while every replica stays ready; " +
				"nothing has failed, so nothing is sent.",
			Quiet: true, MaxMessages: 0, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			nodes := []string{"n1", "n2"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "web",
				"registry.example.com/web:9", 3)
			c.list(w.objects())
			for i := range 3 {
				c.list(w.pod(i, nodes[i%2]))
			}
			c.list(hpaAtMaxAutoscaler(c, 3, 3, 55, false))
			c.after(2 * time.Minute)
			setReplicas(w, 6)
			w.setReady(3)
			c.update(w.objects())
			c.update(hpaAtMaxAutoscaler(c, 3, 6, 140, false))
			for i := 3; i < 6; i++ {
				c.create(w.pod(i, nodes[i%2], startedNow))
			}
			w.setReady(6)
			c.update(w.objects())
			c.after(time.Minute)
			c.update(hpaAtMaxAutoscaler(c, 6, 6, 120, true))
			c.after(40 * time.Minute)
		},
	}
}
