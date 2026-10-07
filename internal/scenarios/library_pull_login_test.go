package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// pullLoginScenarios are registries that refuse the login the failing
// pods pull with, told apart by what the pods pull with.
func pullLoginScenarios() []scenario {
	return []scenario{
		registryLoginExpired(), registryLoginMissingSecret(),
		registryNoPullSecret(),
	}
}

// loginAnswer is a pull error that holds the registry's own words.
const loginAnswer = "failed to authorize: failed to fetch anonymous " +
	"token: 401 Unauthorized: unauthorized: authentication required"

// loginApp is a two-replica Deployment from the private registry that
// pulls with the Secrets named (none for no pull secret).
func loginApp(c *cluster, namespace, name string,
	secrets ...string) *workload {
	w := c.deployment(namespace, name,
		clusterRegistry+"/"+namespace+"/"+name+":4.2", 2)
	if len(secrets) == 0 {
		return w
	}
	configTemplate(w, func(spec *corev1.PodSpec) {
		for _, secret := range secrets {
			spec.ImagePullSecrets = append(spec.ImagePullSecrets,
				corev1.LocalObjectReference{Name: c.n(secret)})
		}
	})
	return w
}

// loginSecret is a registry login Secret last written age ago.
func loginSecret(c *cluster, namespace, name string,
	age time.Duration) *corev1.Secret {
	meta := c.meta(namespace, name)
	meta.CreationTimestamp = metav1.NewTime(c.start.Add(-age))
	return &corev1.Secret{
		ObjectMeta: meta, Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("{}")},
	}
}

// loginOutage lists the apps on two nodes, adds a third at the end of a
// quiet minute and fails every new replica's pull with answer.
func loginOutage(c *cluster, apps []*workload, answer string) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	for _, w := range apps {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	c.after(time.Minute)
	c.create(c.node("n3", "zone-a"))
	for _, w := range apps {
		setReplicas(w, 3)
		w.setReady(2)
		c.update(w.objects())
	}
	for n := range 6 {
		for _, w := range apps {
			clusterPullFailure(c, w, 2, "n3", n, answer)
		}
		c.after(40 * time.Second)
	}
}

// registryLoginExpired: three workloads in two namespaces pull with a
// login Secret nobody has touched for 92 days. A new node cannot pull
// any of their images, and the registry answers with an authentication
// error. The registry is the root, and the message names the Secrets
// and how old they are.
func registryLoginExpired() scenario {
	return scenario{
		expect: expectation{
			Name: "registry-login-expired",
			Description: "Workloads pull with a registry login Secret " +
				"unchanged for 92 days; a new node's pulls are all " +
				"answered \"unauthorized: authentication required\".",
			Root: "registry//" + clusterRegistry, Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n3", "deployment/shop/api",
				"deployment/shop/worker", "deployment/billing/ledger",
				"secret/shop/regcred"},
		},
		build: func(c *cluster) {
			age := 92 * 24 * time.Hour
			c.list(loginSecret(c, "shop", "regcred", age),
				loginSecret(c, "billing", "regcred", age))
			apps := []*workload{
				loginApp(c, "shop", "api", "regcred"),
				loginApp(c, "shop", "worker", "regcred"),
				loginApp(c, "billing", "ledger", "regcred"),
			}
			loginOutage(c, apps, loginAnswer)
		},
	}
}

// registryLoginMissingSecret: the workloads name a pull Secret that does
// not exist, so the kubelet pulls anonymously and the registry refuses.
// The absent Secret is the cause to fix, not the registry.
func registryLoginMissingSecret() scenario {
	return scenario{
		expect: expectation{
			Name: "registry-login-missing-secret",
			Description: "Workloads name an image pull Secret that does " +
				"not exist; their pulls are refused as unauthorized.",
			Root: "secret/shop/regcred", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n3",
				"registry//" + clusterRegistry},
		},
		build: func(c *cluster) {
			apps := []*workload{
				loginApp(c, "shop", "api", "regcred"),
				loginApp(c, "shop", "worker", "regcred"),
			}
			loginOutage(c, apps, loginAnswer)
		},
	}
}

// registryNoPullSecret: the workloads name no pull Secret for a private
// registry. The registry is the root, and the message says the pods
// have no login to refuse.
func registryNoPullSecret() scenario {
	return scenario{
		expect: expectation{
			Name: "registry-no-pull-secret",
			Description: "Workloads of a private registry name no image " +
				"pull Secret; a new node's pulls are refused as " +
				"unauthorized.",
			Root: "registry//" + clusterRegistry, Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n3", "deployment/shop/api",
				"deployment/shop/worker"},
		},
		build: func(c *cluster) {
			apps := []*workload{
				loginApp(c, "shop", "api"), loginApp(c, "shop", "worker"),
			}
			loginOutage(c, apps, loginAnswer)
		},
	}
}
