package scenarios

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// accessScenarios are RBAC changes that take permissions away, and the
// negative case where an RBAC edit happens next to an unrelated crash.
func accessScenarios() []scenario {
	return []scenario{rbacBindingRemoved(), rbacChangeUnrelatedCrash()}
}

// rbacBindingRemoved: the RoleBinding that lets the api ServiceAccount
// read ConfigMaps is deleted; both api replicas crash with "forbidden"
// errors while the other workload of the namespace stays healthy.
func rbacBindingRemoved() scenario {
	return scenario{
		expect: expectation{
			Name: "rbac-binding-removed",
			Description: "A RoleBinding is deleted; the pods of the " +
				"ServiceAccount it bound crash with forbidden errors.",
			Root: "rolebinding/shop/api-config-reader", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api", "node//n1",
				"node//n2"},
		},
		build: func(c *cluster) {
			api, _ := accessFleet(c)
			binding := accessRoleBinding(c, "api-config-reader", "api")
			c.list(binding)
			c.after(time.Minute)
			c.remove(binding)
			c.after(20 * time.Second)
			message := "Error: configmaps \"api-settings\" is forbidden: " +
				"User \"system:serviceaccount:" + c.n("shop") + ":api\" " +
				"cannot get resource \"configmaps\" in API group \"\" in " +
				"the namespace \"" + c.n("shop") + "\""
			accessCrash(c, api, message)
		},
	}
}

// rbacChangeUnrelatedCrash: a Role of the namespace is edited (a new
// Role appears) while the api pods crash on a bad configuration value.
// The RBAC change must not be blamed: nothing was refused.
func rbacChangeUnrelatedCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "rbac-change-unrelated-crash",
			Description: "A Role is created in the namespace while an " +
				"app crashes on its own configuration; no request is " +
				"forbidden.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"role/shop/metrics-reader"},
		},
		build: func(c *cluster) {
			api, _ := accessFleet(c)
			c.after(time.Minute)
			c.create(accessRole(c, "metrics-reader"))
			c.after(20 * time.Second)
			accessCrash(c, api, "Error: parse LOG_LEVEL: unknown level "+
				"\"verbose\"")
		},
	}
}

// accessFleet lists two nodes and two healthy workloads in shop.
func accessFleet(c *cluster) (*workload, *workload) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	api := c.deployment("shop", "api", "registry.example.com/api:4.2", 2)
	web := c.deployment("shop", "web", "registry.example.com/web:1.9", 2)
	for _, w := range []*workload{api, web} {
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	}
	return api, web
}

// accessCrash crash-loops both replicas of w with message.
func accessCrash(c *cluster, w *workload, message string) {
	for restarts := int32(1); restarts <= 5; restarts++ {
		c.update(w.pod(0, "n1", crashLoop(1, "Error", message, restarts)),
			w.pod(1, "n2", crashLoop(1, "Error", message, restarts)))
		w.setReady(0)
		c.update(w.objects())
		c.after(45 * time.Second)
	}
}

func accessRoleBinding(
	c *cluster, name, account string,
) *unstructured.Unstructured {
	meta := clusterMeta(c, "shop", name, "rolebinding")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
		"subjects": []any{map[string]any{"kind": "ServiceAccount",
			"name": account, "namespace": meta.Namespace}},
		"roleRef": map[string]any{"apiGroup": "rbac.authorization.k8s.io",
			"kind": "Role", "name": "config-reader"},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	return u
}

func accessRole(c *cluster, name string) *unstructured.Unstructured {
	meta := clusterMeta(c, "shop", name, "role")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
		"rules": []any{map[string]any{"apiGroups": []any{""},
			"resources": []any{"pods"}, "verbs": []any{"get", "list"}}},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	return u
}
