package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// rcaOperatorScenarios are custom resources whose controller crashes
// while the field manager of their last edit is not the controller's
// Deployment name.
func rcaOperatorScenarios() []scenario {
	return []scenario{
		crController("cr-controller-crash-manager-is-deployment-name",
			"", "A controller crash-loops; its custom resources in "+
				"two namespaces turn Ready=False after an edit.", 2),
		crController("cr-controller-crash-default-manager", "manager",
			"A controller crash-loops; its custom resources turn "+
				"Ready=False, and the controller's field manager is the "+
				"generic name 'manager'.", 2),
		crController("cr-single-resource-default-manager", "manager",
			"A controller crash-loops; its one custom resource turns "+
				"Ready=False after an edit.", 1),
	}
}

const rcaGadgetGroup = "gadgets.acme.dev"

// rcaGadget is a Gadget custom resource of a namespace, last written by
// manager. Ready is False once the controller stopped reconciling.
func rcaGadget(
	c *cluster, namespace, name, manager string, generation int64,
	ready bool, since time.Time,
) *unstructured.Unstructured {
	meta := clusterMeta(c, namespace, name, "gadget")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rcaGadgetGroup + "/v1", "kind": "Gadget",
		"spec":   map[string]any{"size": int64(3)},
		"status": map[string]any{"observedGeneration": int64(1)},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(generation)
	at := metav1.NewTime(c.start.Add(-time.Hour))
	u.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager: manager, Time: &at,
		Operation: metav1.ManagedFieldsOperationUpdate,
	}})
	status, reason, message := "True", "Reconciled", ""
	if !ready {
		status, reason = "False", "Reconciling"
		message = "waiting for the controller to observe generation 2"
	}
	clusterCondition(u, "Ready", status, reason, message, since)
	return u
}

// rcaGrant is the RBAC of a controller: a ClusterRole that lets it
// update the custom group and a ClusterRoleBinding to its account.
func rcaGrant(
	c *cluster, namespace, account string,
) []*unstructured.Unstructured {
	role := clusterMeta(c, "", "gadget-manager", "clusterrole")
	binding := clusterMeta(c, "", "gadget-manager", "crb")
	roleObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"rules": []any{map[string]any{
			"apiGroups": []any{rcaGadgetGroup},
			"resources": []any{"gadgets", "gadgets/status"},
			"verbs":     []any{"get", "list", "watch", "update", "patch"},
		}},
	}}
	roleObj.SetName(role.Name)
	roleObj.SetUID(role.UID)
	bindingObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole", "name": role.Name},
		"subjects": []any{map[string]any{"kind": "ServiceAccount",
			"name": account, "namespace": namespace}},
	}}
	bindingObj.SetName(binding.Name)
	bindingObj.SetUID(binding.UID)
	return []*unstructured.Unstructured{roleObj, bindingObj}
}

// crController crash-loops a controller Deployment's pod and edits
// resources Gadgets (one per team namespace) that then turn not ready.
// The controller runs as an account the cluster lets update the group.
func crController(
	name, manager, description string, resources int,
) scenario {
	return scenario{
		expect: expectation{
			Name: name, Description: description,
			Root: "deployment/gadget-system/gadget-controller",
			Tier: "notify", MaxMessages: 3, MaxPages: 1,
			MustNotBlame: []string{
				"gadget.gadgets.acme.dev/team-a/primary",
				"gadget.gadgets.acme.dev/team-b/primary"},
		},
		build: func(c *cluster) {
			if manager == "" {
				manager = c.n("gadget-controller")
			}
			buildControllerCrash(c, manager, resources)
		},
	}
}

func buildControllerCrash(c *cluster, manager string, resources int) {
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	account := &corev1.ServiceAccount{
		ObjectMeta: c.meta("gadget-system", "gadget-controller")}
	ctl := c.deployment("gadget-system", "gadget-controller",
		"registry.example.com/gadget-controller:0.9", 1)
	configTemplate(ctl, func(spec *corev1.PodSpec) {
		spec.ServiceAccountName = account.Name
	})
	c.list(account)
	for _, object := range rcaGrant(c, c.n("gadget-system"), account.Name) {
		c.list(object)
	}
	c.list(ctl.objects())
	c.list(ctl.pod(0, "n1"))
	spaces := []string{"team-a", "team-b"}[:resources]
	for _, ns := range spaces {
		c.list(rcaGadget(c, ns, "primary", manager, 1, true,
			c.start.Add(-time.Hour)))
	}
	c.after(time.Minute)
	crash := "panic: runtime error: invalid memory address or nil " +
		"pointer dereference [signal SIGSEGV: segmentation violation]"
	for restarts := int32(1); restarts <= 6; restarts++ {
		c.update(ctl.pod(0, "n1", crashLoop(2, "Error", crash, restarts)))
		ctl.setReady(0)
		c.update(ctl.objects())
		if restarts == 2 {
			for _, ns := range spaces {
				c.update(rcaGadget(c, ns, "primary", manager, 2, false,
					c.now))
			}
		}
		c.after(45 * time.Second)
	}
	c.after(5 * time.Minute)
}
