package scenarios

import (
	"time"
)

// startMidOutageMissingSecret: kwatch starts half an hour into an
// outage. The pod has waited on a Secret since then, but the Secret
// finding is stamped with the time kwatch first looked. The Secret is
// still the root: a restart is not "the cause began after the failure".
func startMidOutageMissingSecret() scenario {
	return scenario{
		expect: expectation{
			Name: "start-mid-outage-missing-secret",
			Description: "kwatch starts half an hour into an outage: " +
				"a pod cannot start because its Secret does not exist.",
			Root: "secret/billing/invoicer-creds", Tier: "notify",
			MaxMessages:  1,
			MustNotBlame: []string{"node//n1", "zone//zone-a"},
			SyncAfter:    duration(30 * time.Second),
		},
		build: func(c *cluster) {
			since := c.now.Add(-30 * time.Minute)
			c.list(c.node("n1", "zone-a"))
			inv := c.deployment("billing", "invoicer",
				"registry.example.com/invoicer:2.0", 1)
			inv.rollout(lifecycleSecretEnv(c.n("invoicer-creds")))
			inv.setReady(0)
			c.list(inv.objects())
			c.list(inv.pod(0, "n1", createdAt(since), waiting(
				"CreateContainerConfigError", "secret \""+
					c.n("invoicer-creds")+"\" not found"),
				notReadySince(since)))
			c.after(6 * time.Minute)
		},
	}
}
