package scenarios

import (
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// gap17ClusterScenarios: a cluster-scoped object (webhook, class, role,
// APIService, CRD) changes and workloads of several namespaces fail.
func gap17ClusterScenarios() []scenario {
	return []scenario{
		gap17InjectorMissingImage(), gap17StorageClassRecreated(),
		gap17PriorityClassDeleted(), gap17ClusterRoleEdited(),
		gap17APIServiceCABundle(), gap17CRDVersionRemoved(),
	}
}

// g17UpSpec lists a healthy Deployment of n pods whose spec mutate edits.
func g17UpSpec(
	c *cluster, namespace, name string, n int, mutate func(*corev1.PodSpec),
) *workload {
	w := c.deployment(namespace, name, "registry.example.com/"+name+":1",
		int32(n))
	configTemplate(w, mutate)
	c.list(w.objects())
	for i := range n {
		c.list(w.pod(i, "n1"))
	}
	return w
}

// g17InitPull is a new pod whose injected init container cannot be pulled.
func g17InitPull(image string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		pod.Status.Phase = corev1.PodPending
		pod.Spec.InitContainers = []corev1.Container{{
			Name: "proxy-init", Image: image}}
		pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
			Name: "proxy-init", Image: image,
			State: corev1.ContainerState{Waiting: &corev1.
				ContainerStateWaiting{Reason: "ImagePullBackOff",
				Message: "Back-off pulling image \"" + image + "\""}}}}
	}
}

func g17Injector(
	c *cluster, image string,
) *admissionv1.MutatingWebhookConfiguration {
	ignore, none := admissionv1.Ignore, admissionv1.SideEffectClassNone
	meta := clusterMeta(c, "", "sidecar-injector", "hook")
	meta.Annotations = map[string]string{
		"injector.example.com/init-image": image}
	meta.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager: "kubectl-edit", Operation: metav1.ManagedFieldsOperationUpdate,
		Time: &metav1.Time{Time: c.now}}}
	return &admissionv1.MutatingWebhookConfiguration{ObjectMeta: meta,
		Webhooks: []admissionv1.MutatingWebhook{{
			Name: "inject.example.com", FailurePolicy: &ignore,
			SideEffects:             &none,
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Namespace: c.n("mesh"), Name: c.n("injector")}}}}}
}

func gap17InjectorMissingImage() scenario {
	return scenario{
		expect: g2Exp("mutating-webhook-injects-missing-image",
			"A config edit makes a mutating webhook inject an init "+
				"container with an image that does not exist; new pods "+
				"of three namespaces stay ImagePullBackOff on a "+
				"container that is in no workload spec.",
			"mutatingwebhookconfiguration//sidecar-injector", "notify",
			"deployment/shop/api", "deployment/billing/ledger",
			"deployment/jobs/runner").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			good := "registry.example/proxy-init:1.0"
			bad := "registry.example/proxy-init:9.9"
			c.list(g17Injector(c, good))
			apps := []*workload{g17Up(c, "shop", "api", 4),
				g17Up(c, "billing", "ledger", 4), g17Up(c, "jobs", "runner", 3)}
			c.after(10 * time.Minute)
			c.update(g17Injector(c, bad))
			c.after(time.Minute)
			g17Loop(c, 6, func(round int32) {
				for _, app := range apps {
					n := int(*app.deployment.Spec.Replicas)
					g17State(c, app, n, g17InitPull(bad))
					gapKubelet(c, app.pod(0, "n1"), "Failed",
						"Failed to pull image \""+bad+"\": not found", round)
				}
			})
		},
	}
}

func gap17StorageClassRecreated() scenario {
	return scenario{
		expect: g2Exp("storageclass-recreated-wrong-parameters",
			"A StorageClass is deleted and recreated with an invalid "+
				"parameter; new claims of three namespaces stay Pending "+
				"on a provisioning failure while bound volumes work.",
			"storageclass//fast-ssd", "notify", "deployment/shop/api",
			"persistentvolumeclaim/shop/cache").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			class := func(kind string) *storagev1.StorageClass {
				sc := scheduleClass(c, "fast-ssd",
					storagev1.VolumeBindingImmediate)
				sc.Parameters = map[string]string{"type": kind}
				return sc
			}
			c.list(class("gp3"))
			api := g17Up(c, "shop", "api", 2)
			c.after(10 * time.Minute)
			c.remove(class("gp3"))
			c.after(time.Second)
			c.create(class("gp9"))
			var claims []*corev1.PersistentVolumeClaim
			for _, ref := range [][2]string{{"shop", "cache"},
				{"data", "pg"}, {"reports", "store"}, {"shop", "uploads"}} {
				claim := scheduleClaim(c, ref[0], ref[1], "fast-ssd")
				claims = append(claims, claim)
				c.create(claim)
			}
			g17Loop(c, 6, func(round int32) {
				for _, claim := range claims {
					c.warn(c.warningEvent(claim, "PersistentVolumeClaim",
						"ProvisioningFailed", "failed to provision volume "+
							"with StorageClass \"fast-ssd\": invalid "+
							"parameter type", "pd.csi.storage.gke.io", round))
				}
				g17State(c, api, 1, pendingUnscheduled("0/1 nodes are "+
					"available: pod has unbound immediate "+
					"PersistentVolumeClaims."))
			})
		},
	}
}

func gap17PriorityClassDeleted() scenario {
	return scenario{
		expect: g2Exp("priorityclass-deleted-replicasets-refused",
			"A PriorityClass is deleted while Deployments of three "+
				"namespaces name it; at their next rollouts the "+
				"ReplicaSets cannot create pods, and the old pods keep "+
				"serving.",
			"priorityclass//team-high", "notify", "deployment/shop/api",
			"deployment/billing/api", "deployment/jobs/runner").
			budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			class := g17Obj(c, "scheduling.k8s.io/v1", "PriorityClass", "",
				"team-high", map[string]any{"value": int64(1000)})
			c.list(class)
			var apps []*workload
			for _, app := range [][2]string{{"shop", "api"},
				{"billing", "api"}, {"jobs", "runner"}} {
				apps = append(apps, g17UpSpec(c, app[0], app[1], 2,
					func(spec *corev1.PodSpec) {
						spec.PriorityClassName = c.n("team-high")
					}))
			}
			c.after(10 * time.Minute)
			c.remove(class)
			c.after(25 * time.Minute)
			g17FailedCreate(c, apps, "pods are forbidden: no PriorityClass "+
				"with name "+c.n("team-high")+" was found")
		},
	}
}

func gap17ClusterRoleEdited() scenario {
	return scenario{
		expect: g2Exp("clusterrole-edit-agents-forbidden-three-namespaces",
			"A ClusterRole bound in three namespaces loses the "+
				"endpointslices resource; the agents there are denied "+
				"listing it and exit on their next restart.",
			"clusterrole//app-discovery", "notify", "deployment/shop/api",
			"deployment/billing/api", "deployment/jobs/runner").
			budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			role := func(resources ...any) *unstructured.Unstructured {
				return rbacObject(c, "ClusterRole", "", "app-discovery",
					map[string]any{"rules": []any{map[string]any{
						"apiGroups": []any{"", "discovery.k8s.io"},
						"resources": resources,
						"verbs":     []any{"get", "list", "watch"}}}})
			}
			c.list(role("pods", "endpointslices"))
			var apps []*workload
			for _, app := range [][2]string{{"shop", "api"},
				{"billing", "api"}, {"jobs", "runner"}} {
				c.list(&corev1.ServiceAccount{
					ObjectMeta: c.meta(app[0], "agent")})
				c.list(rbacObject(c, "RoleBinding", app[0], "app-discovery",
					map[string]any{"subjects": []any{map[string]any{
						"kind": "ServiceAccount", "name": "agent",
						"namespace": c.n(app[0])}},
						"roleRef": map[string]any{
							"apiGroup": "rbac.authorization.k8s.io",
							"kind":     "ClusterRole",
							"name":     c.n("app-discovery")}}))
				apps = append(apps, g17UpSpec(c, app[0], app[1], 2,
					func(spec *corev1.PodSpec) {
						spec.ServiceAccountName = "agent"
					}))
			}
			c.after(10 * time.Minute)
			c.update(role("pods"))
			c.after(time.Minute)
			g17Loop(c, 6, func(round int32) {
				g17Victims(c, apps, round, "endpointslices.discovery."+
					"k8s.io is forbidden: cannot list resource "+
					"\"endpointslices\" in API group \"discovery.k8s.io\"")
			})
		},
	}
}

func gap17APIServiceCABundle() scenario {
	return scenario{
		expect: g2Exp("apiservice-cabundle-mismatch-discovery-fails",
			"An APIService fails its discovery check on an unknown "+
				"certificate authority after its backend's serving "+
				"certificate rotated; the backend is Ready and two "+
				"namespace deletions stick.",
			"apiservice//v1alpha1.custom.example.com", "notify",
			"deployment/custom-system/backend", "namespace//old-shop",
			"namespace//old-jobs").budget(1).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			g17Up(c, "custom-system", "backend", 1)
			api := func(status string) *unstructured.Unstructured {
				u := g17Obj(c, "apiregistration.k8s.io/v1", "APIService", "",
					"v1alpha1.custom.example.com", map[string]any{
						"spec": map[string]any{"group": "custom.example.com",
							"version": "v1alpha1", "service": map[string]any{
								"namespace": c.n("custom-system"),
								"name":      c.n("backend")}}})
				clusterCondition(u, "Available", status, "FailedDiscoveryCheck",
					"x509: certificate signed by unknown authority", c.now)
				return u
			}
			c.list(api("True"))
			c.after(10 * time.Minute)
			c.update(api("False"))
			c.after(15 * time.Minute)
			for _, name := range []string{"old-shop", "old-jobs"} {
				ns := &corev1.Namespace{ObjectMeta: c.meta("", name)}
				deleting(&ns.ObjectMeta, c.now)
				ns.Spec.Finalizers = []corev1.FinalizerName{"kubernetes"}
				ns.Status.Phase = corev1.NamespaceTerminating
				ns.Status.Conditions = []corev1.NamespaceCondition{{
					Type:   corev1.NamespaceDeletionDiscoveryFailure,
					Status: corev1.ConditionTrue, Reason: "DiscoveryFailed",
					Message: "Discovery failed for some groups, 1 failing: " +
						"custom.example.com/v1alpha1: x509: certificate " +
						"signed by unknown authority",
					LastTransitionTime: metav1.NewTime(c.now)}}
				c.create(ns)
			}
			c.after(30 * time.Minute)
		},
	}
}

func gap17CRDVersionRemoved() scenario {
	return scenario{
		expect: g2Exp("crd-version-removed-operators-log-no-match",
			"A CRD upgrade stops serving its old version; operators of "+
				"three namespaces log that no match exists for the kind "+
				"in that version and crash-loop.",
			"customresourcedefinition//widgets.example.com", "notify",
			"deployment/shop/widget-op", "deployment/billing/widget-op",
			"deployment/reports/widget-op").budget(1).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			crd := func(oldServed bool) *unstructured.Unstructured {
				return g17Obj(c, "apiextensions.k8s.io/v1",
					"CustomResourceDefinition", "", "widgets.example.com",
					map[string]any{"spec": map[string]any{
						"group": "example.com", "versions": []any{
							map[string]any{"name": "v1beta1",
								"served": oldServed},
							map[string]any{"name": "v1", "served": true}}}})
			}
			c.list(crd(true))
			apps := g17Apps(c, 2, "shop/widget-op", "billing/widget-op",
				"reports/widget-op")
			c.after(10 * time.Minute)
			c.update(crd(false))
			c.after(time.Minute)
			g17Loop(c, 6, func(round int32) {
				g17Victims(c, apps, round, "no matches for kind \"Widget\" "+
					"in version \"example.com/v1beta1\"")
			})
		},
	}
}
