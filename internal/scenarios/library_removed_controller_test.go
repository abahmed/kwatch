package scenarios

import (
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// removedControllerScenarios are the leftovers of controllers that were
// removed: each controller's objects are one digest incident rooted at
// it, never one per object, and never one with a live failure.
func removedControllerScenarios() []scenario {
	return []scenario{
		removedMeshLeftovers(), removedPolicyEngineLeftovers(),
		removedRunnerLeftovers(), removedOperatorOneGroup(),
		parkedRoutesOneLine(), removedMeshBesideLiveFailure(),
		youngStuckPeerJoinsOldGroup(),
	}
}

// leftAge is how long the leftovers have been left: weeks, younger
// than the old-problem horizon, so each would be an incident alone.
const leftAge = 20 * 24 * time.Hour

// removedMeshLeftovers: a service mesh control plane was removed. Its
// disruption budget selects nothing, its autoscaler targets the gone
// Deployment and its sidecar injector calls a Service with no pods:
// one digest incident rooted at the gone Deployment.
func removedMeshLeftovers() scenario {
	return scenario{
		expect: expectation{
			Name: "removed-mesh-leftovers",
			Description: "A removed mesh control plane left a budget " +
				"that selects nothing, an autoscaler of its gone " +
				"Deployment and an injector webhook with no pods: " +
				"one digest incident rooted at the Deployment.",
			Root: "deployment/mesh-system/mesh-control",
			Tier: "digest", MaxMessages: 1, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			listMeshLeftovers(c)
			c.after(time.Minute)
		},
	}
}

// listMeshLeftovers lists what the removed mesh left in mesh-system.
func listMeshLeftovers(c *cluster) {
	c.list(clusterService(c, "mesh-system", "mesh-control", 443),
		clusterSlice(c, "mesh-system", "mesh-control"))
	c.list(emptyBudget(c, "mesh-system", "mesh-control", "mesh-control"))
	c.list(missingTargetHPA(c, "mesh-system", "mesh-control",
		"mesh-control"))
	c.list(ignoringMutatingHook(c, "mesh-sidecar-injector",
		"sidecar.mesh.example.com", "mesh-system", "mesh-control"))
}

// removedMeshBesideLiveFailure: the same leftovers, and a Deployment of
// the namespace that crash-loops now. The crash keeps its own
// incident; it never joins the leftovers.
func removedMeshBesideLiveFailure() scenario {
	return scenario{
		expect: expectation{
			Name: "removed-mesh-beside-live-failure",
			Description: "Leftovers of a removed mesh beside a " +
				"Deployment of the namespace that crash-loops: the " +
				"crash is an incident of its own.",
			Root: "deployment/mesh-system/mesh-gateway",
			OtherRoots: []string{
				"deployment/mesh-system/mesh-control"},
			Tier: "notify", MaxMessages: 3, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			listMeshLeftovers(c)
			gateway := c.deployment("mesh-system", "mesh-gateway",
				"registry.example.com/mesh-gateway:1.0", 1)
			c.list(gateway.objects())
			c.list(gateway.pod(0, "n1"))
			c.after(time.Minute)
			gateway.setReady(0)
			c.update(gateway.objects())
			c.update(gateway.pod(0, "n1",
				crashLoop(2, "Error", "panic: listener failed", 5)))
			c.after(10 * time.Minute)
		},
	}
}

// removedPolicyEngineLeftovers: a policy engine was uninstalled. Four
// webhooks that ignore failure call its gone Service, its Helm release
// is stuck uninstalling and its serving certificate expired: one
// digest incident rooted at the Service.
func removedPolicyEngineLeftovers() scenario {
	return scenario{
		expect: expectation{
			Name: "removed-policy-engine-leftovers",
			Description: "An uninstalled policy engine left four " +
				"webhooks calling its gone Service, its Helm release " +
				"stuck uninstalling and an expired certificate: one " +
				"digest incident rooted at the Service.",
			Root: "service/guard-system/guard-svc",
			Tier: "digest", MaxMessages: 1, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			for _, name := range []string{"guard-validate",
				"guard-mutate", "guard-cleanup", "guard-exceptions"} {
				c.list(ignoringHook(clusterValidatingHook(c, name,
					name+".guard.example.com", "guard-system",
					"guard-svc")))
			}
			c.list(agedTLSSecret(c, "guard-system", "guard-svc.tls",
				leftAge))
			c.helmReleaseAged("guard-system", "guard", leftAge,
				"deployed", "uninstalling")
			c.after(time.Minute)
		},
	}
}

// removedRunnerLeftovers: a runner controller was removed. Its runner
// set and four objects of the runners stay stuck deleting on its
// finalizers, and its webhook calls a Service with no pods: one digest
// incident rooted at the Service.
func removedRunnerLeftovers() scenario {
	return scenario{
		expect: expectation{
			Name: "removed-runner-leftovers",
			Description: "A removed runner controller left a runner " +
				"set and four objects stuck deleting on its " +
				"finalizers and a webhook with no pods: one digest " +
				"incident rooted at its webhook Service.",
			Root: "service/runner-system/runner-webhook",
			Tier: "digest", MaxMessages: 1, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(clusterService(c, "runner-system", "runner-webhook",
				443), clusterSlice(c, "runner-system", "runner-webhook"))
			c.list(ignoringMutatingHook(c, "runner-mutation",
				"mutate-runner.runs.example.com", "runner-system",
				"runner-webhook"))
			const group = "runs.example.com"
			c.list(stuckObject(c, "runners", group+"/v1", "RunnerSet",
				"set-a", group+"/runner-set-protection"))
			for _, o := range []struct{ api, kind, name string }{
				{"v1", "ServiceAccount", "set-a-sa"},
				{"rbac.authorization.k8s.io/v1", "Role", "set-a-role"},
				{"rbac.authorization.k8s.io/v1", "RoleBinding",
					"set-a-binding"},
				{group + "/v1", "Listener", "set-a-listener"},
			} {
				c.list(stuckObject(c, "runners", o.api, o.kind, o.name,
					group+"/cleanup-protection"))
			}
			c.after(time.Minute)
		},
	}
}

// removedOperatorOneGroup: an operator was removed and left two webhook
// Services, each without pods: one webhook calls each. Both are its
// leftovers: one digest incident, not two.
func removedOperatorOneGroup() scenario {
	return scenario{
		expect: expectation{
			Name: "removed-operator-one-group",
			Description: "A removed operator left two webhook Services " +
				"without pods, each called by its webhooks and " +
				"named after it: one digest incident, not two.",
			Root: "service/ops-system/data-operator-conversion",
			Tier: "digest", MaxMessages: 1, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			for _, svc := range []string{"data-operator-webhook",
				"data-operator-conversion"} {
				c.list(clusterService(c, "ops-system", svc, 443),
					clusterSlice(c, "ops-system", svc))
				for _, hook := range []string{"a", "b"} {
					name := svc + "-" + hook
					c.list(ignoringHook(clusterValidatingHook(c, name,
						name+".data.example.com", "ops-system", svc)))
				}
			}
			c.after(time.Minute)
		},
	}
}

// parkedRoutesOneLine: three Deployments of one namespace were scaled
// to zero weeks ago and their Ingresses still route to them: one
// digest incident for the namespace that lists the three.
func parkedRoutesOneLine() scenario {
	return scenario{
		expect: expectation{
			Name: "parked-routes-one-line",
			Description: "Three Deployments of one namespace were " +
				"scaled to zero weeks ago while Ingresses still " +
				"route to them: one digest incident for the " +
				"namespace.",
			Root: "namespace//demo",
			Tier: "digest", MaxMessages: 1, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			for _, name := range []string{"cart", "search", "wishlist"} {
				w := c.deployment("demo", name,
					"registry.example.com/"+name+":1", 0)
				w.deployment.ManagedFields = writtenAgo(c, leftAge)
				c.list(w.objects())
				c.list(clusterService(c, "demo", name, 8080),
					clusterSlice(c, "demo", name))
				ing := scaleDownIngress(c, name)
				ing.Namespace = c.n("demo")
				c.list(ing)
			}
			c.after(time.Minute)
		},
	}
}

// writtenAgo is the managed fields of a spec written age ago.
func writtenAgo(c *cluster, age time.Duration) []metav1.ManagedFieldsEntry {
	at := metav1.NewTime(c.now.Add(-age))
	return []metav1.ManagedFieldsEntry{{Manager: "kubectl",
		Operation: metav1.ManagedFieldsOperationUpdate, Time: &at,
		FieldsType: "FieldsV1",
		FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{}}`)}}}
}

// emptyBudget is a disruption budget of namespace selecting app=value
// that has found no pod for leftAge.
func emptyBudget(
	c *cluster, namespace, name, value string,
) *policyv1.PodDisruptionBudget {
	min := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: clusterMeta(c, namespace, name, "pdb"),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &min,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app": c.n(value)}},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			Conditions: []metav1.Condition{{
				Type: "DisruptionAllowed", Status: "False",
				Reason:             "InsufficientPods",
				LastTransitionTime: metav1.NewTime(c.now.Add(-leftAge)),
			}},
		},
	}
}

// missingTargetHPA is an autoscaler of namespace whose Deployment
// target has been gone for leftAge.
func missingTargetHPA(
	c *cluster, namespace, name, target string,
) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := clusterHPA(c, namespace, name, "True", "ValidMetricFound", "")
	hpa.Spec.ScaleTargetRef.Name = c.n(target)
	hpa.Status.Conditions[0] = autoscalingv2.
		HorizontalPodAutoscalerCondition{
		Type: autoscalingv2.AbleToScale, Status: "False",
		Reason: "FailedGetScale",
		Message: "the HPA controller was unable to get the target's " +
			"current scale: deployments.apps \"" + c.n(target) +
			"\" not found",
		LastTransitionTime: metav1.NewTime(c.now.Add(-leftAge)),
	}
	return hpa
}

// ignoringMutatingHook is a mutating webhook configuration that skips
// calls it cannot make.
func ignoringMutatingHook(
	c *cluster, name, webhook, namespace, service string,
) *admissionv1.MutatingWebhookConfiguration {
	config := admissionMutatingHook(c, name, webhook, namespace, service,
		10)
	ignore := admissionv1.Ignore
	config.Webhooks[0].FailurePolicy = &ignore
	return config
}
