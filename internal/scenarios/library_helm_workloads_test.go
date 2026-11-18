package scenarios

import (
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// helmWorkloadScenarios are failed Helm releases next to failing
// workloads: linked when the release deployed the workloads that broke
// after it, apart otherwise.
func helmWorkloadScenarios() []scenario {
	return []scenario{helmUpgradeBreaksWorkload(),
		helmUpgradeFailsBesideOtherCrash(),
		helmUpgradeFailsAfterCrash()}
}

// helmUpgradeBreaksWorkload: revision 12 of release payments rolls out
// a new image of its Deployment; the new pods crash-loop, and after the
// wait times out Helm marks the revision failed. One incident, rooted
// at the release.
func helmUpgradeBreaksWorkload() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-upgrade-breaks-workload",
			Description: "A Helm upgrade rolls out an image whose pods " +
				"crash-loop and then fails: the release and its " +
				"workload are one incident.",
			Root: "helmrelease/shop/payments", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//n1"},
			Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			w := helmWorkload(c, "payments")
			c.after(2 * time.Minute)
			began := c.now
			helmRevisionsAt(c, "shop", "payments", began, time.Time{},
				upgradeTo("pending-upgrade")...)
			rolloutStart(c, w, "payments")
			// The upgrade's wait gives up after a minute.
			c.after(20 * time.Second)
			helmRevisionsAt(c, "shop", "payments", began, c.now,
				upgradeTo("failed")...)
			crashOn(c, w)
		},
	}
}

// helmUpgradeFailsBesideOtherCrash: release payments fails while its
// own workload stays healthy, and Deployment orders of another release
// in the same namespace crash-loops after its own rollout. Two
// incidents: the release is not blamed for orders.
func helmUpgradeFailsBesideOtherCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-upgrade-fails-beside-other-crash",
			Description: "A Helm release fails while a workload of " +
				"another release in its namespace crash-loops: two " +
				"separate incidents.",
			Root: "helmrelease/shop/payments", Tier: "notify",
			OtherRoots:  []string{"deployment/shop/orders"},
			MaxMessages: 4,
			MustNotBlame: []string{"node//n1",
				"helmrelease/shop/orders"},
			Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			helmWorkload(c, "payments")
			orders := helmWorkload(c, "orders")
			c.after(2 * time.Minute)
			helmRevisionsAt(c, "shop", "payments", c.now, c.now,
				upgradeTo("failed")...)
			c.after(time.Minute)
			rolloutCrash(c, orders, "orders")
		},
	}
}

// helmUpgradeFailsAfterCrash: Deployment payments crash-loops after a
// rollout; ten minutes later an upgrade of its release fails on a hook
// without touching it. The crash began before the revision: two
// incidents.
func helmUpgradeFailsAfterCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "helm-upgrade-fails-after-crash",
			Description: "A workload crash-loops; a later Helm upgrade " +
				"of its release fails: the release is not blamed for " +
				"the earlier crash.",
			Root: "deployment/shop/payments", Tier: "notify",
			OtherRoots:  []string{"helmrelease/shop/payments"},
			MaxMessages: 4, MustNotBlame: []string{"node//n1"},
			Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			w := helmWorkload(c, "payments")
			c.after(2 * time.Minute)
			rolloutCrash(c, w, "payments")
			c.after(10 * time.Minute)
			helmRevisionsAt(c, "shop", "payments", c.now, c.now,
				upgradeTo("failed")...)
		},
	}
}

// helmWorkload lists node n1 and a serving Deployment app in shop that
// Helm release app deploys, with the release's eleven revisions, the
// last deployed.
func helmWorkload(c *cluster, app string) *workload {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", app, "registry.example.com/"+app+":1.0", 1)
	w.deployment.Labels = mergeLabels(w.deployment.Labels,
		map[string]string{"app.kubernetes.io/instance": c.n(app)})
	w.deployment.Annotations = mergeLabels(w.deployment.Annotations,
		map[string]string{helmReleaseAnnotation: c.n(app)})
	c.list(w.objects())
	c.list(w.pod(0, "n1"))
	helmRevisionsAt(c, "shop", app, c.now, time.Time{},
		deployedHistory(11)...)
	return w
}

// rolloutCrash rolls out image 1.1 of app, whose pod crash-loops.
func rolloutCrash(c *cluster, w *workload, app string) {
	rolloutStart(c, w, app)
	crashOn(c, w)
}

// rolloutStart rolls out image 1.1 of app; its pod crashes at start.
func rolloutStart(c *cluster, w *workload, app string) {
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image = "registry.example.com/" + app + ":1.1"
	})
	c.update(w.deployment)
	c.create(rs)
	c.create(w.pod(0, "n1", startedNow, notReady))
	c.after(40 * time.Second)
	c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
		"panic: missing key DB_URL", 3)))
}

// crashOn keeps the new pod of w crash-looping two minutes later.
func crashOn(c *cluster, w *workload) {
	c.after(2 * time.Minute)
	c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
		"panic: missing key DB_URL", 5)))
}

// deployedHistory is the statuses of a release's revisions 1 to n,
// oldest first, after n successful upgrades: the last is deployed.
func deployedHistory(n int) []string {
	out := make([]string, 0, n+1)
	for range n - 1 {
		out = append(out, "superseded")
	}
	return append(out, "deployed")
}

// helmRevisionsAt delivers release name whose revisions have the given
// statuses, oldest first. The latest was created at created; modified,
// when set, is when Helm gave it its status (its modifiedAt label).
func helmRevisionsAt(
	c *cluster, ns, name string, created, modified time.Time,
	statuses ...string,
) {
	ns, name = c.n(ns), c.n(name)
	var revisions []kube.HelmRevision
	for i, status := range statuses {
		meta := &metav1.ObjectMeta{Namespace: ns,
			Name: "sh.helm.release.v1." + name + ".v" +
				strconv.Itoa(i+1),
			CreationTimestamp: metav1.NewTime(created.Add(-time.Hour)),
			Labels: map[string]string{"owner": "helm", "name": name,
				"status": status, "version": strconv.Itoa(i + 1)}}
		if i == len(statuses)-1 {
			meta.CreationTimestamp = metav1.NewTime(created)
			if !modified.IsZero() {
				meta.Labels["modifiedAt"] = strconv.FormatInt(
					modified.Unix(), 10)
			}
		}
		if r, ok := kube.HelmRevisionOf(meta); ok {
			revisions = append(revisions, r)
		}
	}
	c.emit(kube.HelmReleaseObservation(
		kube.HelmReleaseID(ns, name), revisions, c.now))
}

// upgradeTo is the statuses of a release upgraded from eleven deployed
// revisions to a twelfth in status.
func upgradeTo(status string) []string {
	return append(deployedHistory(11), status)
}
