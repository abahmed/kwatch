package scenarios

import (
	"time"
)

// helmOldScenarios are Helm releases whose latest revision failed long
// before kwatch started: old news while what they deploy is healthy.
func helmOldScenarios() []scenario {
	return []scenario{helmOldFailureHealthy(), helmOldFailureWorkloadDown()}
}

// helmFailedLongAgo is how long before the start the latest revision
// of the release failed: past the 30-day old-problem horizon.
const helmFailedLongAgo = 60 * 24 * time.Hour

// helmOldFailureHealthy: revision 12 of release payments failed two
// months ago (a hook that was never retried); its Deployment serves.
// The release is an old problem: listed, not announced.
func helmOldFailureHealthy() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-old-failure-healthy",
			Description: "A Helm release failed two months before " +
				"kwatch started and its workload is healthy: an old " +
				"problem, not a warning.",
			Quiet: true, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			helmWorkload(c, "payments")
			failed := c.now.Add(-helmFailedLongAgo)
			helmRevisionsAt(c, "shop", "payments", failed, failed,
				upgradeTo("failed")...)
			c.after(time.Minute)
		},
	}
}

// helmOldFailureWorkloadDown is the twin: the release failed two months
// ago, and now its Deployment rolls out an image that crash-loops. The
// crash is its own incident, and the release, with a workload down, is
// not old news.
func helmOldFailureWorkloadDown() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-old-failure-workload-down",
			Description: "A Helm release failed two months ago; now " +
				"its workload crash-loops after a rollout: the release " +
				"is not blamed for the new crash, nor hidden.",
			Root: "deployment/shop/payments", Tier: "notify",
			OtherRoots:  []string{"helmrelease/shop/payments"},
			MaxMessages: 4, MustNotBlame: []string{"node//n1"},
			Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			w := helmWorkload(c, "payments")
			failed := c.now.Add(-helmFailedLongAgo)
			helmRevisionsAt(c, "shop", "payments", failed, failed,
				upgradeTo("failed")...)
			c.after(time.Minute)
			rolloutCrash(c, w, "payments")
		},
	}
}
