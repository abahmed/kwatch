package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// dependencyConfigScenarios are workloads whose environment names a
// dependency through a ConfigMap instead of a literal value.
func dependencyConfigScenarios() []scenario {
	return []scenario{configHostRefuses(), configHostInCredentialKey()}
}

// configHostRefuses: the orders API reads DB_HOST from a ConfigMap
// (envFrom) and crash-loops; kwatch's probe of that host is refused. The
// host is the root and the message says where it was read.
func configHostRefuses() scenario {
	return scenario{
		expect: expectation{
			Name: "dependency-host-from-configmap",
			Description: "A Deployment crash-loops on a database whose " +
				"address only a ConfigMap names; the probe is refused.",
			Root: "external-endpoint//" + ordersDatabase, Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"deployment/orders/api", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			api := configApp(c, "DB_HOST")
			c.after(time.Minute)
			db := inventory.CoreID(kube.KindExternalEndpoint, "",
				ordersDatabase)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.probe(db, "dial tcp: connection refused")
				dependencyCrash(c, api, restarts)
				c.after(time.Minute)
			}
		},
	}
}

// configHostInCredentialKey: the same address sits in a key named like a
// credential, which kwatch never reads. The workload stays the root.
func configHostInCredentialKey() scenario {
	return scenario{
		expect: expectation{
			Name: "dependency-host-in-credential-key",
			Description: "A Deployment crash-loops; the only address its " +
				"ConfigMap holds is under a key named like a credential.",
			Root: "deployment/orders/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"external-endpoint//" + ordersDatabase,
				"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			api := configApp(c, "DB_PASSWORD")
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				dependencyCrash(c, api, restarts)
				c.after(time.Minute)
			}
		},
	}
}

// scenarioDigestKey keys the ConfigMap digests so the logs are stable.
var scenarioDigestKey = []byte("scenario-digest-key")

// configApp lists the orders API with envFrom of ConfigMap api-config,
// whose key holds the database address. The ConfigMap goes through the
// informer transform, as in a cluster.
func configApp(c *cluster, key string) *workload {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "api-config",
			Namespace: "orders"},
		Data: map[string]string{key: ordersDatabase, "LOG_LEVEL": "info"},
	}
	cached, err := kube.InformerTransform(scenarioDigestKey)(cm)
	if err != nil {
		panic(err)
	}
	c.list(cached.(*corev1.ConfigMap))
	api := c.deployment("orders", "api", "registry.example.com/api:3.1", 2)
	api.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "api-config"}}}}
	})
	api.setReady(2)
	c.list(api.objects())
	c.list(api.pod(0, "n1"), api.pod(1, "n1"))
	return api
}
