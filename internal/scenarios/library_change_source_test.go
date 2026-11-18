package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// changeSourceScenarios are bad releases whose workload records where
// it was built from; the message links the release to its source only
// when what is recorded is valid.
func changeSourceScenarios() []scenario {
	return []scenario{releaseSourceLinked(), releaseSourceInvalid()}
}

// sourceSHA is the commit a CI pipeline recorded on the release.
const sourceSHA = "4f9c2a1b7d3e5f60718293a4b5c6d7e8f9012345"

// sourceRelease rolls out revision 14 of checkout, a new image whose
// pods crash-loop, with annotations set on the Deployment as its
// pipeline recorded them.
func sourceRelease(c *cluster, labels, annotations map[string]string) {
	w := revisionBase(c, "", nil)
	c.after(5 * time.Minute)
	w.deployment.Labels = labels
	w.deployment.Annotations = annotations
	numberedRollout(c, w, "argocd-controller", "14",
		func(spec *corev1.PodSpec) {
			spec.Containers[0].Image = "registry.example.com/checkout:5.2"
		})
	releaseUndoCrashLoop(c, w)
}

// releaseSourceLinked: an Argo CD application deploys checkout, and its
// pipeline records the repository and commit on the Deployment. The
// message names the application (no web address is configured) and
// links the commit on its known host.
func releaseSourceLinked() scenario {
	return scenario{
		expect: expectation{
			Name: "release-source-linked",
			Description: "A GitOps release whose Deployment records its " +
				"repository and commit crash-loops; the message links " +
				"the commit and names the application.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			sourceRelease(c,
				map[string]string{"argocd.argoproj.io/instance": "shop"},
				map[string]string{
					"org.opencontainers.image.source": "https://github.com/" +
						"example/checkout.git",
					"org.opencontainers.image.revision": sourceSHA,
				})
		},
	}
}

// releaseSourceInvalid: the same release, but the recorded repository
// carries credentials and the revision is not a commit or tag. Neither
// is shown nor linked, and there is no source line.
func releaseSourceInvalid() scenario {
	return scenario{
		expect: expectation{
			Name: "release-source-invalid",
			Description: "A release whose recorded repository has " +
				"credentials and whose revision is not a commit " +
				"crash-loops; the message has no source line.",
			Root: "deployment/shop/checkout", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//n1", "node//n2"},
			Tail:         duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			sourceRelease(c, nil, map[string]string{
				"org.opencontainers.image.source": "https://ci:token@" +
					"github.com/example/checkout",
				"org.opencontainers.image.revision": "build 14 of main",
			})
		},
	}
}
