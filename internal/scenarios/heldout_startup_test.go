package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// heldoutStartupScenarios are held-out pods that fail as they start.
// They were written on 2026-10-01, when four earlier held-out scenarios
// became labelled, and their labels were fixed before they were first
// replayed. See heldoutLibrary for the rule they follow.
func heldoutStartupScenarios() []scenario {
	return []scenario{heldoutPullSecretRotated(),
		heldoutLivenessTooAggressive()}
}

// heldoutPullSecretRotated: automation rotates the payments namespace's
// image pull Secret and writes a token that the private registry does
// not accept. Running pods are unaffected, but when the ledger scales
// from 2 to 4 the new replicas fail to pull with 401 Unauthorized. The
// risk namespace pulls from the same registry with its own pull Secret
// and scales out fine at the same time. The rotated Secret is the root;
// the registry works.
func heldoutPullSecretRotated() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-pull-secret-rotated",
			Description: "A namespace's image pull Secret is rotated to " +
				"a token the registry rejects; new replicas fail to pull " +
				"with 401 while another namespace pulls fine.",
			Root: "secret/payments/pull-creds", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"registry//" + heldoutRegistry,
				"deployment/payments/ledger", "deployment/risk/scorer",
				"node//p1", "node//p2"},
		},
		build: buildHeldoutPullSecret,
	}
}

// heldoutRegistry is the private registry of the pull-secret scenario.
const heldoutRegistry = "images.internal.example"

func buildHeldoutPullSecret(c *cluster) {
	c.list(c.node("p1", "zone-a"), c.node("p2", "zone-b"))
	ledger := heldoutPrivateApp(c, "payments", "ledger", "pull-creds")
	scorer := heldoutPrivateApp(c, "risk", "scorer", "registry-auth")
	c.list(heldoutPullSecret(c, "payments", "pull-creds", "token-v7"),
		heldoutPullSecret(c, "risk", "registry-auth", "token-r2"))
	for _, w := range []*workload{ledger, scorer} {
		c.list(w.objects())
		c.list(w.pod(0, "p1"), w.pod(1, "p2"))
	}
	c.after(2 * time.Minute)
	c.update(heldoutPullSecret(c, "payments", "pull-creds", "token-v8"))
	c.after(6 * time.Minute)
	setReplicas(scorer, 3)
	scorer.setReady(2)
	c.update(scorer.objects())
	c.create(scorer.pod(2, "p1", startedNow))
	scorer.setReady(3)
	c.update(scorer.objects())
	setReplicas(ledger, 4)
	ledger.setReady(2)
	c.update(ledger.objects())
	refused := "failed to authorize: failed to fetch anonymous token: " +
		"unexpected status from GET request to https://" +
		heldoutRegistry + "/v2/token: 401 Unauthorized"
	for attempt := range 6 {
		for i, node := range []string{"p1", "p2"} {
			clusterPullFailure(c, ledger, i+2, node, attempt, refused)
		}
		c.after(40 * time.Second)
	}
}

// heldoutPrivateApp is a two-replica Deployment pulling from the private
// registry with secret.
func heldoutPrivateApp(c *cluster, namespace, name,
	secret string) *workload {
	w := c.deployment(namespace, name,
		heldoutRegistry+"/"+namespace+"/"+name+":3.1", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.ImagePullSecrets = []corev1.LocalObjectReference{{
			Name: c.n(secret),
		}}
	})
	return w
}

// heldoutPullSecret is a docker-config pull Secret holding token.
func heldoutPullSecret(c *cluster, namespace, name,
	token string) *corev1.Secret {
	config := `{"auths":{"` + heldoutRegistry + `":{"auth":"` + token +
		`"}}}`
	return &corev1.Secret{
		ObjectMeta: c.meta(namespace, name),
		Type:       corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(config),
		},
	}
}

// heldoutLivenessTooAggressive: the report renderer warms a large cache
// before it listens, which takes about 70 seconds on a fresh node. Its
// liveness probe starts after 10 seconds and allows two failures 5
// seconds apart, so a new replica is killed long before it can answer.
// The renderer scales from 2 to 5 for month-end; the two old replicas
// keep running, every new one restarts again and again. The
// Deployment's own probe configuration is the root, not the nodes.
func heldoutLivenessTooAggressive() scenario {
	return scenario{
		expect: expectation{
			Name: "heldout-liveness-too-aggressive",
			Description: "New replicas of a slow-starting app are killed " +
				"by a liveness probe that gives up after 20 seconds, " +
				"while running replicas needed 70 seconds to start.",
			Root: "deployment/reports/renderer", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//q1", "node//q2", "node//q3"},
		},
		build: buildHeldoutLiveness,
	}
}

func buildHeldoutLiveness(c *cluster) {
	nodes := []string{"q1", "q2", "q3"}
	for _, node := range nodes {
		c.list(c.node(node, "zone-a"))
	}
	w := c.deployment("reports", "renderer",
		"registry.example.com/renderer:12.4", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Ports = []corev1.ContainerPort{{
			Name: "http", ContainerPort: 8080,
		}}
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path: "/live", Port: intstr.FromInt32(8080)}},
			InitialDelaySeconds: 10, PeriodSeconds: 5, FailureThreshold: 2,
		}
	})
	c.list(w.objects())
	c.list(w.pod(0, "q1", readyAfter(70*time.Second)),
		w.pod(1, "q2", readyAfter(68*time.Second)))
	c.after(3 * time.Minute)
	setReplicas(w, 5)
	w.setReady(2)
	c.update(w.objects())
	killed := "Liveness probe failed: Get \"http://10.244.6.21:8080/live\":" +
		" dial tcp 10.244.6.21:8080: connect: connection refused"
	for restarts := int32(1); restarts <= 5; restarts++ {
		for i := 2; i < 5; i++ {
			pod := w.pod(i, nodes[i%3], startedNow,
				crashLoop(143, "Error", "", restarts))
			pod.CreationTimestamp = metav1.NewTime(c.now.Add(
				-time.Duration(restarts) * 45 * time.Second))
			c.update(pod)
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy", killed,
				"kubelet", restarts*2))
		}
		c.after(45 * time.Second)
	}
}
