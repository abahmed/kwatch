package scenarios

import (
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// helmScenarios are Helm releases whose latest revision failed or stays
// pending, and the states that must stay quiet.
func helmScenarios() []scenario {
	return []scenario{helmUpgradeFailed(), helmPendingStuck(),
		helmDeployedQuiet(), helmPendingShortQuiet()}
}

// helmUpgradeFailed: revision 3 of release payments failed; revision 2
// was deployed before it. Its pods still run the old revision.
func helmUpgradeFailed() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-upgrade-failed",
			Description: "The latest revision of a Helm release is " +
				"failed; the release is reported once.",
			Root: "helmrelease/shop/payments", Tier: "notify",
			MaxMessages: 2, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			buildHelm(c, 0, "deployed", "superseded", "failed")
		},
	}
}

// helmPendingStuck: an upgrade has stayed pending-upgrade for 25 minutes
// when kwatch first lists the release.
func helmPendingStuck() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-pending-upgrade-stuck",
			Description: "A Helm upgrade stays pending-upgrade far " +
				"longer than an upgrade takes.",
			Root: "helmrelease/shop/payments", Tier: "notify",
			MaxMessages: 2, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			buildHelm(c, 25*time.Minute,
				"superseded", "deployed", "pending-upgrade")
		},
	}
}

// helmDeployedQuiet: an upgrade that succeeded leaves the older
// revisions superseded; nothing is wrong.
func helmDeployedQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-upgrade-deployed-quiet",
			Description: "A Helm upgrade succeeded: deployed and " +
				"superseded revisions raise nothing.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			buildHelm(c, 0, "superseded", "superseded", "deployed")
		},
	}
}

// helmPendingShortQuiet: an upgrade is pending for a few minutes, then
// deploys, as an upgrade that waits for its pods does.
func helmPendingShortQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-pending-upgrade-short-quiet",
			Description: "A Helm upgrade is pending for five minutes " +
				"and then deploys; nothing is reported.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			buildHelm(c, 0, "superseded", "deployed", "pending-upgrade")
			c.after(5 * time.Minute)
			c.helmRelease("shop", "payments",
				"superseded", "superseded", "deployed")
		},
	}
}

// buildHelm lists a running workload of release payments and its
// release, whose revisions have the given statuses, oldest first; the
// latest got its status age ago.
func buildHelm(c *cluster, age time.Duration, statuses ...string) {
	c.list(c.node("n1", "zone-a"))
	api := c.deployment("shop", "payments", "ghcr.io/x/payments:2.0", 2)
	api.setReady(2)
	api.deployment.Labels = map[string]string{
		"app.kubernetes.io/instance": c.n("payments")}
	c.list(api.objects())
	c.list(api.pod(0, "n1"), api.pod(1, "n1"))
	c.after(time.Minute)
	c.helmReleaseAged("shop", "payments", age, statuses...)
}

// helmRelease delivers what the release Secret watch reports for a
// release whose revisions have the given statuses, oldest first. Each
// status is dated now: the revision changed when it was delivered.
func (c *cluster) helmRelease(ns, name string, statuses ...string) {
	c.helmReleaseAged(ns, name, 0, statuses...)
}

// helmReleaseAged is helmRelease with the latest revision's status set
// age ago.
func (c *cluster) helmReleaseAged(
	ns, name string, age time.Duration, statuses ...string,
) {
	ns, name = c.n(ns), c.n(name)
	var revisions []kube.HelmRevision
	for i, status := range statuses {
		meta := &metav1.ObjectMeta{Namespace: ns,
			Name: "sh.helm.release.v1." + name + ".v" +
				strconv.Itoa(i+1),
			CreationTimestamp: metav1.NewTime(c.now),
			Labels: map[string]string{"owner": "helm", "name": name,
				"status": status, "version": strconv.Itoa(i + 1)}}
		if i == len(statuses)-1 {
			meta.CreationTimestamp = metav1.NewTime(c.now.Add(-age))
		}
		if r, ok := kube.HelmRevisionOf(meta); ok {
			revisions = append(revisions, r)
		}
	}
	c.emit(kube.HelmReleaseObservation(
		kube.HelmReleaseID(ns, name), revisions, c.now))
}
