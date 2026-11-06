package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// releaseReportScenarios are releases that went wrong on workloads a
// GitOps controller or Helm deploys: the message says to undo the
// release there, not with a rollback in the cluster that the next sync
// would overwrite.
func releaseReportScenarios() []scenario {
	return []scenario{
		managedBadRelease("release-gitops-managed", "orders",
			map[string]string{"argocd.argoproj.io/instance": "shop"}, nil),
		managedBadRelease("release-helm-managed", "billing", nil,
			map[string]string{"meta.helm.sh/release-name": "billing"}),
	}
}

// managedBadRelease rolls out a new image whose pods crash-loop on a
// Deployment carrying labels and annotations of the tool that deploys
// it; the previous revision keeps serving.
func managedBadRelease(
	name, app string, labels, annotations map[string]string,
) scenario {
	return scenario{
		expect: expectation{
			Name: name,
			Description: "A Deployment that a deployment tool manages " +
				"rolls out an image whose pods crash-loop; the previous " +
				"revision keeps serving.",
			Root: "deployment/shop/" + app, Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", app,
				"registry.example.com/"+app+":1.0", 2)
			w.deployment.Labels = mergeLabels(w.deployment.Labels, labels)
			w.deployment.Annotations = mergeLabels(
				w.deployment.Annotations, annotations)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(70 * time.Second)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/" +
					app + ":1.1"
			})
			c.update(w.deployment)
			c.create(rs)
			c.create(w.pod(0, "n1", startedNow, notReady))
			c.after(40 * time.Second)
			c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
				"panic: missing key DB_URL", 3)))
			c.after(2 * time.Minute)
			c.update(w.pod(0, "n1", startedNow, crashLoop(1, "Error",
				"panic: missing key DB_URL", 5)))
		},
	}
}

// mergeLabels is base with extra added.
func mergeLabels(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
