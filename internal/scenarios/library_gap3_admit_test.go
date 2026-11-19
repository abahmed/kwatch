package scenarios

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gap3AdmitScenarios are quotas, limits and admission rules that refuse
// the pods or objects a controller other than a ReplicaSet creates.
func gap3AdmitScenarios() []scenario {
	return []scenario{
		gap3PSAStatefulSetJob(), gap3StorageQuotaClaim(),
		gap3QuotaMustSpecify(), gap3ScopedQuota(), gap3LimitRatio(),
		gap3PSAVersion(), gap3PolicyParamDeleted(), gap3EphemeralQuota(),
		gap3ReplicaSetQuota(),
	}
}

const gap3PSAMessage = "violates PodSecurity \"restricted:latest\": " +
	"allowPrivilegeEscalation != false (container \"app\" must set " +
	"securityContext.allowPrivilegeEscalation=false), unrestricted " +
	"capabilities (container \"app\" must set securityContext.capabilities." +
	"drop=[\"ALL\"]), runAsNonRoot != true (pod or container \"app\" must " +
	"set securityContext.runAsNonRoot=true), seccompProfile (pod or " +
	"container \"app\" must set securityContext.seccompProfile.type to " +
	"\"RuntimeDefault\" or \"Localhost\")"

// gap3Quota is a ResourceQuota of ns with hard and used limits.
func gap3Quota(
	c *cluster, namespace, name string, hard, used corev1.ResourceList,
) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: clusterMeta(c, namespace, name, "quota"),
		Spec:       corev1.ResourceQuotaSpec{Hard: hard},
		Status:     corev1.ResourceQuotaStatus{Hard: hard, Used: used},
	}
}

// gap3PSAStatefulSetJob: a restricted label is put on the queue's
// namespace; the scale-up of its StatefulSet and a compaction Job both
// fail to create pods, and no pod exists to look at.
func gap3PSAStatefulSetJob() scenario {
	return scenario{
		expect: g2Exp("pod-security-refuses-statefulset-and-job",
			"A namespace gets an enforced restricted Pod Security label; "+
				"a StatefulSet scale-up and a Job both fail with "+
				"\"violates PodSecurity\" and create no pod.",
			"namespace//streaming", "notify", "node//n1",
			"statefulset/streaming/queue").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			ns := namespaceOf(c, "streaming", nil)
			c.list(ns)
			cron := scheduleCronJob(c, "streaming", "compact", "0 * * * *")
			c.list(cron)
			c.after(10 * time.Minute)
			labelled := last(c, ns)
			labelled.Labels = map[string]string{
				"pod-security.kubernetes.io/enforce": "restricted"}
			c.update(labelled)
			c.after(time.Minute)
			scaled := sts.DeepCopy()
			replicas := int32(4)
			scaled.Spec.Replicas = &replicas
			scaled.Generation, scaled.Status.ObservedGeneration = 2, 2
			c.update(scaled)
			job := activeJob(c, cron, "compact-29251930", c.now)
			c.create(job)
			for m := int32(1); m <= 5; m++ {
				g2Event(c, scaled, "StatefulSet", "FailedCreate", "create "+
					"Pod queue-3 in StatefulSet queue failed error: pods "+
					"\"queue-3\" is forbidden: "+gap3PSAMessage,
					"statefulset-controller", m)
				g2Event(c, job, "Job", "FailedCreate", "Error creating: "+
					"pods \""+job.Name+"-\" is forbidden: "+gap3PSAMessage,
					"job-controller", m)
				c.after(time.Minute)
			}
		},
	}
}

// gap3StorageQuotaClaim: a StatefulSet grows from three to four brokers;
// the storage quota of the namespace refuses the fourth claim, so no pod
// is created.
func gap3StorageQuotaClaim() scenario {
	return scenario{
		expect: g2Exp("statefulset-claim-refused-by-storage-quota",
			"A StatefulSet scale-up cannot create its fourth "+
				"PersistentVolumeClaim: the namespace's storage quota is "+
				"used up; no pod is created.",
			"resourcequota/streaming/storage-quota", "notify",
			"statefulset/streaming/queue", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			quota := gap3Quota(c, "streaming", "storage-quota",
				corev1.ResourceList{corev1.ResourceRequestsStorage: resource.
					MustParse("150Gi")}, corev1.ResourceList{
					corev1.ResourceRequestsStorage: resource.MustParse(
						"150Gi")})
			c.list(quota)
			c.after(10 * time.Minute)
			scaled := sts.DeepCopy()
			replicas := int32(4)
			scaled.Spec.Replicas = &replicas
			scaled.Generation, scaled.Status.ObservedGeneration = 2, 2
			c.update(scaled)
			for m := int32(1); m <= 6; m++ {
				g2Event(c, scaled, "StatefulSet", "FailedCreate", "create "+
					"Claim data-queue-3 for Pod queue-3 in StatefulSet "+
					"queue failed error: persistentvolumeclaims "+
					"\"data-queue-3\" is forbidden: exceeded quota: "+
					"storage-quota, requested: requests.storage=50Gi, "+
					"used: requests.storage=150Gi, limited: requests."+
					"storage=150Gi", "statefulset-controller", m)
				c.after(time.Minute)
			}
		},
	}
}

// gap3QuotaMustSpecify: a platform team adds a CPU quota to the shop
// namespace; the next rollout of api, whose pods set no resources, is
// refused because the quota requires them.
func gap3QuotaMustSpecify() scenario {
	return g2Release(g2Exp("quota-requires-requests-and-limits",
		"A compute ResourceQuota is added to a namespace; the next "+
			"rollout, whose pods set no requests or limits, is refused "+
			"with \"must specify limits.cpu\".",
		"resourcequota/shop/compute-quota", "notify", "node//n1",
		"deployment/shop/api").tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].Env = []corev1.EnvVar{{Name: "LOG",
				Value: "debug"}}
		},
		func(c *cluster, w *workload) {
			c.list(gap3Quota(c, "shop", "compute-quota",
				corev1.ResourceList{
					corev1.ResourceRequestsCPU: resource.MustParse("8"),
					corev1.ResourceLimitsCPU:   resource.MustParse("16")},
				corev1.ResourceList{
					corev1.ResourceRequestsCPU: resource.MustParse("1"),
					corev1.ResourceLimitsCPU:   resource.MustParse("2")}))
			g2FailedCreate("Error creating: pods \""+w.replicaSet.Name+
				"-\" is forbidden: failed quota: compute-quota: must "+
				"specify limits.cpu for: app; requests.cpu for: app")(c, w)
		})
}

// gap3ScopedQuota: a quota that limits pods of the high PriorityClass to
// two is used up; the third replica of web, which sets that class, is
// refused although the namespace has room.
func gap3ScopedQuota() scenario {
	return scenario{
		expect: g2Exp("quota-scoped-to-priority-class-exhausted",
			"A ResourceQuota scoped to one PriorityClass allows two "+
				"pods; a Deployment's third replica of that class is "+
				"refused.",
			"resourcequota/shop/high-priority-pods", "notify",
			"node//n1", "deployment/shop/web").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.PriorityClassName = "high"
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			pods := corev1.ResourceList{corev1.ResourcePods: resource.
				MustParse("2")}
			quota := gap3Quota(c, "shop", "high-priority-pods", pods, pods)
			quota.Spec.ScopeSelector = &corev1.ScopeSelector{
				MatchExpressions: []corev1.ScopedResourceSelectorRequirement{{
					ScopeName: corev1.ResourceQuotaScopePriorityClass,
					Operator:  corev1.ScopeSelectorOpIn,
					Values:    []string{"high"}}}}
			c.list(quota)
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			g2FailedCreate("Error creating: pods \""+w.replicaSet.Name+
				"-\" is forbidden: exceeded quota: high-priority-pods, "+
				"requested: pods=1, used: pods=2, limited: pods=2")(c, w)
		},
	}
}

// gap3LimitRatio: a LimitRange caps the limit to request ratio of CPU at
// 2; a rollout that sets limit 4 and request 500m is refused.
func gap3LimitRatio() scenario {
	return g2Release(g2Exp("limit-range-ratio-refuses-rollout",
		"A rollout whose CPU limit is eight times its request is "+
			"refused by the namespace's LimitRange, which allows a "+
			"ratio of two.",
		"limitrange/shop/limits", "notify", "node//n1",
		"deployment/shop/api").tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].Resources = corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m")},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4")}}
		},
		func(c *cluster, w *workload) {
			c.list(&corev1.LimitRange{
				ObjectMeta: clusterMeta(c, "shop", "limits", "lr"),
				Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
					Type: corev1.LimitTypeContainer,
					MaxLimitRequestRatio: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("2")}}}}})
			g2FailedCreate("Error creating: pods \""+w.replicaSet.Name+
				"-\" is forbidden: cpu max limit to request ratio per "+
				"Container is 2, but provided ratio is 8.000000")(c, w)
		})
}

// gap3PSAVersion: after a cluster upgrade three namespaces labelled
// restricted:latest refuse the next rollout of their apps for a field the
// new version requires.
func gap3PSAVersion() scenario {
	e := g2Exp("pod-security-latest-version-three-namespaces",
		"After a cluster upgrade three namespaces enforce "+
			"restricted:latest; the next rollout of each is refused "+
			"for a newly required field.",
		"namespace//team-a", "notify", "node//n1").budget(4).
		tail(10 * time.Minute)
	e.OtherRoots = []string{"namespace//team-b", "namespace//team-c"}
	return scenario{
		expect: e,
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			var fleet []*workload
			for _, team := range []string{"team-a", "team-b", "team-c"} {
				c.list(namespaceOf(c, team, map[string]string{
					"pod-security.kubernetes.io/enforce":         "restricted",
					"pod-security.kubernetes.io/enforce-version": "latest"}))
				w := c.deployment(team, "app", "registry.example.com/app:1", 2)
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n1"))
				fleet = append(fleet, w)
			}
			c.after(10 * time.Minute)
			for _, w := range fleet {
				rs := w.rollout(func(spec *corev1.PodSpec) {
					spec.Containers[0].Image = "registry.example.com/app:2"
				})
				d := w.deployment
				d.Status.UpdatedReplicas = 0
				d.Status.Conditions = []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentReplicaFailure, Status: "True",
					Reason:             "FailedCreate",
					Message:            gap3PSAMessage,
					LastTransitionTime: metav1.NewTime(c.now)}}
				c.update(d)
				c.create(rs)
			}
			for m := int32(1); m <= 5; m++ {
				for _, w := range fleet {
					g2Event(c, w.replicaSet, "ReplicaSet", "FailedCreate",
						"Error creating: pods \""+w.replicaSet.Name+
							"-\" is forbidden: "+gap3PSAMessage,
						"replicaset-controller", m)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap3PolicyParamDeleted: the ConfigMap a ValidatingAdmissionPolicy
// binding reads its parameters from is deleted; with parameterNotFound
// Action Deny, the policy denies every pod in the three namespaces it
// matches.
func gap3PolicyParamDeleted() scenario {
	return scenario{
		expect: g2Exp("admission-policy-param-deleted-denies-all",
			"The parameter ConfigMap of a ValidatingAdmissionPolicy "+
				"binding is deleted; rollouts in three namespaces are "+
				"refused with \"no params found for policy binding\".",
			"validatingadmissionpolicy//require-owner.example.com",
			"notify", "node//n1", "deployment/shop/api").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(admissionPolicy(c, "require-owner.example.com"))
			var fleet []*workload
			for _, ns := range []string{"shop", "billing", "media"} {
				w := c.deployment(ns, "app", "registry.example.com/app:1", 2)
				c.list(w.objects())
				c.list(w.pod(0, "n1"), w.pod(1, "n1"))
				fleet = append(fleet, w)
			}
			c.after(10 * time.Minute)
			message := "Error creating: pods is forbidden: " +
				"ValidatingAdmissionPolicy 'require-owner.example.com' " +
				"with binding 'require-owner-binding' denied request: " +
				"failed to configure binding: no params found for policy " +
				"binding with `Deny` parameterNotFoundAction"
			for _, w := range fleet {
				rs := w.rollout(func(spec *corev1.PodSpec) {
					spec.Containers[0].Image = "registry.example.com/app:2"
				})
				c.update(w.deployment)
				c.create(rs)
			}
			for m := int32(1); m <= 5; m++ {
				for _, w := range fleet {
					g2Event(c, w.replicaSet, "ReplicaSet", "FailedCreate",
						message, "replicaset-controller", m)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap3EphemeralQuota: a pod with a generic ephemeral volume waits for the
// claim the ephemeral volume controller must create; the namespace's
// storage quota refuses it.
func gap3EphemeralQuota() scenario {
	return scenario{
		expect: g2Exp("ephemeral-volume-claim-refused-by-quota",
			"A pod with a generic ephemeral volume stays Pending "+
				"waiting for its claim, which the namespace's storage "+
				"quota refuses.",
			"resourcequota/shop/storage-quota", "notify", "node//n1",
			"deployment/shop/api").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			storage := corev1.ResourceList{
				corev1.ResourceRequestsStorage: resource.MustParse("100Gi")}
			c.list(gap3Quota(c, "shop", "storage-quota", storage, storage))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 1)
			w.setReady(0)
			c.create(w.objects())
			message := "0/1 nodes are available: waiting for ephemeral " +
				"volume controller to create the persistentvolumeclaim " +
				"\"" + w.pod(0, "").Name + "-scratch\". preemption: 0/1 " +
				"nodes are available: 1 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			pendingForMinutes(c, w, message, 8)
		},
	}
}

// gap3ReplicaSetQuota: the object-count quota for ReplicaSets is full, so
// a rollout cannot even create its new ReplicaSet; the old pods serve.
func gap3ReplicaSetQuota() scenario {
	return scenario{
		expect: g2Exp("deployment-replicaset-create-refused-by-quota",
			"A rollout cannot create its new ReplicaSet because the "+
				"namespace's object-count quota is full; the old pods "+
				"keep serving.",
			"resourcequota/shop/object-counts", "notify", "node//n1",
			"deployment/shop/api").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:2", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			count := corev1.ResourceList{"count/replicasets.apps": resource.
				MustParse("10")}
			c.list(gap3Quota(c, "shop", "object-counts", count, count))
			c.after(10 * time.Minute)
			message := "Failed to create new replica set \"api-5d9c\": " +
				"replicasets.apps \"api-5d9c\" is forbidden: exceeded " +
				"quota: object-counts, requested: count/replicasets." +
				"apps=1, used: count/replicasets.apps=10, limited: " +
				"count/replicasets.apps=10"
			d := w.deployment.DeepCopy()
			d.Spec.Template.Spec.Containers[0].Image =
				"registry.example.com/api:3"
			d.Generation++
			d.Status.Conditions = []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentProgressing, Status: "False",
				Reason: "ReplicaSetCreateError", Message: message,
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(d)
			for m := int32(1); m <= 6; m++ {
				g2Event(c, d, "Deployment", "ReplicaSetCreateError",
					message, "deployment-controller", m)
				c.after(time.Minute)
			}
		},
	}
}
