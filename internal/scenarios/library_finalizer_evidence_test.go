package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// finalizerEvidenceScenarios say who handles a finalizer only from what
// ties a workload to it: a look-alike name proves nothing, a role that
// writes the group or the owner of the finalizer does.
func finalizerEvidenceScenarios() []scenario {
	return []scenario{
		finalizerLookAlikeControllers(), finalizerLookAlikeNoRBACView(),
		finalizerControllerGrantedTheGroup(),
		finalizerOwnedByAWorkload(),
	}
}

const (
	// arcAge is how long the runner leftovers have been stuck: months.
	arcAge = 300 * 24 * time.Hour
	// arcNamespace is where the operator and its leftovers live.
	arcNamespace = "arc-system"
	// arcCleanup is the finalizer of the leftovers, in the API group
	// actions.github.com.
	arcCleanup = "actions.github.com/cleanup-protection"
)

// finalizerLookAlikeControllers: four objects of an operator that is long
// gone were deleted ten months ago and wait for a finalizer of the group
// actions.github.com. Two scaled-down Deployments are named like that
// group's controller, but their ServiceAccount may write only another
// group (actions.summerwind.dev). They are not blamed: the finalizer is.
func finalizerLookAlikeControllers() scenario {
	return scenario{
		expect: expectation{
			Name: "finalizer-lookalike-controllers",
			Description: "Four objects have been stuck deleting for " +
				"months on a finalizer of one API group; two " +
				"scaled-down Deployments are named like its controller " +
				"but their role writes another group.",
			Root: "finalizer/arc-system/" + arcCleanup,
			Tier: "digest", MaxMessages: 2,
			Tail: duration(40 * time.Minute),
			MustNotBlame: []string{
				"deployment/arc-system/actions-runner-controller",
				"deployment/arc-system/" +
					"actions-runner-controller-github-webhook-server"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			arcLookAlikes(c)
			c.list(arcLeftovers(c, "")...)
		},
	}
}

// finalizerLookAlikeNoRBACView: the same look-alikes when kwatch sees no
// role at all. With nothing that ties them to the group they are not
// blamed either.
func finalizerLookAlikeNoRBACView() scenario {
	return scenario{
		expect: expectation{
			Name: "finalizer-lookalike-no-rbac-view",
			Description: "Four objects have been stuck deleting for " +
				"months on a finalizer of one API group; two " +
				"scaled-down Deployments are named like its controller " +
				"and nothing else is known about them.",
			Root: "finalizer/arc-system/" + arcCleanup,
			Tier: "digest", MaxMessages: 2,
			Tail: duration(40 * time.Minute),
			MustNotBlame: []string{
				"deployment/arc-system/actions-runner-controller",
				"deployment/arc-system/" +
					"actions-runner-controller-github-webhook-server"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			for _, name := range arcLookAlikeNames {
				c.list(arcScaledDown(c, name).objects())
			}
			c.list(arcLeftovers(c, "")...)
		},
	}
}

// finalizerControllerGrantedTheGroup: the positive twin. The scaled-down
// Deployment runs as a ServiceAccount whose role may write the group
// actions.github.com, so it is the controller of the finalizer.
func finalizerControllerGrantedTheGroup() scenario {
	return scenario{
		expect: expectation{
			Name: "finalizer-controller-granted-the-group",
			Description: "Four objects have been stuck deleting for " +
				"months on a finalizer of one API group; the " +
				"scaled-down Deployment whose role writes that group " +
				"is its controller.",
			Root: "deployment/arc-system/gha-runner-scale-set-controller",
			Tier: "digest", MaxMessages: 2,
			Tail: duration(40 * time.Minute),
			MustNotBlame: []string{
				"deployment/arc-system/actions-runner-controller"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			arcLookAlikes(c)
			w := arcScaledDown(c, "gha-runner-scale-set-controller")
			c.list(w.objects())
			c.list(arcRBAC(c, w.deployment.Name, "actions.github.com")...)
			c.list(arcLeftovers(c, "")...)
		},
	}
}

// finalizerOwnedByAWorkload: the field manager that put the finalizer on
// the objects is a Deployment of another namespace, found by its exact
// name; it is scaled down and is blamed.
func finalizerOwnedByAWorkload() scenario {
	const manager = "gha-finalizer-reaper"
	return scenario{
		expect: expectation{
			Name: "finalizer-owned-by-a-workload",
			Description: "Four objects have been stuck deleting for " +
				"months; the field manager that owns their finalizer " +
				"is a scaled-down Deployment of another namespace.",
			Root: "deployment/tools/" + manager,
			Tier: "digest", MaxMessages: 2,
			Tail: duration(40 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(c.deployment("tools", manager,
				"registry.example.com/reaper:1", 0).objects())
			c.list(arcLeftovers(c, manager)...)
		},
	}
}

// arcLookAlikeNames are the two Deployments of the operator of another
// group, named like the controller of actions.github.com.
var arcLookAlikeNames = []string{
	"actions-runner-controller",
	"actions-runner-controller-github-webhook-server",
}

// arcScaledDown is a Deployment of the operator namespace scaled to
// zero, running as a ServiceAccount of its own name.
func arcScaledDown(c *cluster, name string) *workload {
	w := c.deployment(arcNamespace, name,
		"registry.example.com/"+name+":0.27", 0)
	w.deployment.Spec.Template.Spec.ServiceAccountName = name
	return w
}

// arcLookAlikes lists the look-alike Deployments with the role of the
// other group: they may write actions.summerwind.dev and nothing else.
func arcLookAlikes(c *cluster) {
	for _, name := range arcLookAlikeNames {
		c.list(arcScaledDown(c, name).objects())
		c.list(arcRBAC(c, name, "actions.summerwind.dev")...)
	}
}

// arcRBAC is a ServiceAccount and the ClusterRole and ClusterRoleBinding
// that let it update and patch the group, all named name.
func arcRBAC(c *cluster, name, group string) []runtime.Object {
	account := &corev1.ServiceAccount{
		ObjectMeta: c.meta(arcNamespace, name)}
	role := rbacObject(c, "ClusterRole", "", name, map[string]any{
		"rules": []any{
			map[string]any{"apiGroups": []any{group},
				"resources": []any{"runners", "runners/finalizers"},
				"verbs":     []any{"get", "list", "update", "patch"}},
			map[string]any{"apiGroups": []any{""},
				"resources": []any{"secrets"},
				"verbs":     []any{"get", "list"}},
		}})
	binding := rbacObject(c, "ClusterRoleBinding", "", name,
		map[string]any{
			"roleRef": map[string]any{"kind": "ClusterRole", "name": c.n(name),
				"apiGroup": "rbac.authorization.k8s.io"},
			"subjects": []any{map[string]any{"kind": "ServiceAccount",
				"name": c.n(name), "namespace": c.n(arcNamespace)}},
		})
	return []runtime.Object{account, role, binding}
}

// rbacObject is a built-in RBAC object with the given top-level fields.
func rbacObject(
	c *cluster, kind, namespace, name string, fields map[string]any,
) *unstructured.Unstructured {
	meta := clusterMeta(c, namespace, name, kind)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": kind}}
	for key, value := range fields {
		u.Object[key] = value
	}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	return u
}

// arcLeftovers are four objects of the operator namespace that were
// deleted arcAge ago and still wait for arcCleanup: a Secret, a
// ServiceAccount, a Role and a RoleBinding. When manager is not empty it
// is the field manager that owns the finalizer of each.
func arcLeftovers(c *cluster, manager string) []runtime.Object {
	var out []runtime.Object
	for _, o := range []struct{ apiVersion, kind, name string }{
		{"v1", "Secret", "test-gha-rs-github-secret"},
		{"v1", "ServiceAccount", "test-gha-rs-no-permission"},
		{"rbac.authorization.k8s.io/v1", "Role", "test-gha-rs-manager"},
		{"rbac.authorization.k8s.io/v1", "RoleBinding", "test-gha-rs-manager"},
	} {
		u := arcLeftover(c, o.apiVersion, o.kind, o.name)
		if manager != "" {
			u.SetManagedFields([]metav1.ManagedFieldsEntry{{
				Manager: manager, Time: timeAt(c, -arcAge),
				Operation: metav1.ManagedFieldsOperationUpdate,
				FieldsV1: &metav1.FieldsV1{Raw: []byte(
					`{"f:metadata":{"f:finalizers":{"v:\"` +
						arcCleanup + `\"":{}}}}`)}}})
		}
		out = append(out, u)
	}
	return out
}

func timeAt(c *cluster, offset time.Duration) *metav1.Time {
	at := metav1.NewTime(c.now.Add(offset))
	return &at
}

// arcLeftover is an object of the operator namespace deleted arcAge ago
// and held by arcCleanup.
func arcLeftover(
	c *cluster, apiVersion, kind, name string,
) *unstructured.Unstructured {
	meta := clusterMeta(c, arcNamespace, name, kind)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion, "kind": kind}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetDeletionTimestamp(timeAt(c, -arcAge))
	u.SetFinalizers([]string{arcCleanup})
	return u
}
