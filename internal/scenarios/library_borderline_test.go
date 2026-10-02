package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// borderlineScenarios are labelled cases with ambiguous evidence: two
// plausible upstream changes close together, partial evidence, or weak
// timing. The label is the true root, decided from what happened in the
// cluster, not from what the engine is expected to conclude. They exist
// so the "likely" confidence level has enough cases to be calibrated:
// a level is only honest if some of its cases are hard.
func borderlineScenarios() []scenario {
	return []scenario{
		borderlinePolicyAndConfig(), borderlineSelectorAndRollout(),
		borderlineOperatorUpgradeAndEdit(), borderlineScheduleAndSecret(),
		borderlineLateConfigReload(),
	}
}

// borderlinePolicyAndConfig: within ten seconds someone edits the api's
// ConfigMap (a log format change the app ignores) and creates an egress
// policy that only allows DNS. The pods then time out reaching their
// database. Both changes touch the api; the policy is the root.
func borderlinePolicyAndConfig() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-policy-and-config",
			Description: "A ConfigMap edit and an egress NetworkPolicy " +
				"land together; the pods then time out reaching their " +
				"database.",
			Root: "networkpolicy/payments/restrict-egress", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1", "node//n2"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			cm := configMap(c, "payments", "api-config",
				map[string]string{"log_format": "text"})
			api := c.deployment("payments", "api",
				"registry.example.com/pay:6", 2)
			configTemplate(api, func(spec *corev1.PodSpec) {
				spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cm.Name,
						},
					},
				}}
			})
			c.list(cm)
			c.list(api.objects())
			c.list(api.pod(0, "n1"), api.pod(1, "n2"))
			c.after(time.Minute)
			edited := last(c, cm)
			edited.Data["log_format"] = "json"
			c.update(edited)
			c.after(10 * time.Second)
			c.create(clusterEgressPolicy(c, api))
			c.after(20 * time.Second)
			message := "FATAL: failed to connect to `host=10.0.3.4 " +
				"user=payments database=payments`: dial error (dial tcp " +
				"10.0.3.4:5432: i/o timeout)"
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(api.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), api.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				api.setReady(0)
				c.update(api.objects())
				c.after(45 * time.Second)
			}
		},
	}
}

// borderlineSelectorAndRollout: a release rolls out a new image, and a
// minute later the Service selector is edited with a typo. The new pods
// are healthy but the Service selects nothing. The Service edit is the
// root; the rollout, which replaced every endpoint a minute earlier, is
// the plausible alternative.
func borderlineSelectorAndRollout() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-selector-and-rollout",
			Description: "A healthy rollout and a Service selector edit " +
				"land a minute apart; the selector has a typo and the " +
				"Service has no endpoints.",
			Root: "service/shop/checkout", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "checkout",
				"registry.example.com/checkout:7", 2)
			c.list(w.objects())
			old := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(old[0], old[1])
			svc := clusterService(c, "shop", "checkout", 8080)
			c.list(svc, trafficSlice(c, "checkout", old...))
			c.after(time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/checkout:8"
			})
			c.update(w.deployment)
			c.create(rs)
			fresh := []*corev1.Pod{w.pod(0, "n1", startedNow),
				w.pod(1, "n1", startedNow)}
			c.create(fresh[0], fresh[1])
			w.setReady(2)
			c.update(w.objects())
			c.remove(last(c, old[0]), last(c, old[1]))
			c.update(trafficSlice(c, "checkout", fresh...))
			c.after(time.Minute)
			edited := last(c, svc)
			edited.Spec.Selector = map[string]string{
				"app": c.n("checkuot"),
			}
			c.update(edited)
			c.update(trafficSlice(c, "checkout"))
			c.after(5 * time.Minute)
		},
	}
}

// borderlineOperatorUpgradeAndEdit: the Postgres operator is upgraded
// (its new pod is healthy) and two minutes later the orders-db resource
// is edited to shrink its volume, which the operator refuses. The edit
// is the root; the operator upgrade is the plausible alternative.
func borderlineOperatorUpgradeAndEdit() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-operator-upgrade-and-edit",
			Description: "An operator upgrade and an invalid custom " +
				"resource edit land two minutes apart; the resource " +
				"reports ReconcileError.",
			Root: operatorCRRoot, Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			operator := c.deployment("db-operators", "pg-operator",
				"registry.example.com/pg-operator:2.4", 1)
			c.list(operator.objects())
			c.list(operator.pod(0, "n1"))
			c.list(clusterPostgres(c, 3, "100Gi", 3, "True", "Reconciled",
				""))
			c.after(time.Minute)
			oldPod := last(c, operator.pod(0, ""))
			rs := operator.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image =
					"registry.example.com/pg-operator:2.5"
			})
			c.update(operator.deployment)
			c.create(rs)
			c.create(operator.pod(0, "n1", startedNow))
			operator.setReady(1)
			c.update(operator.objects())
			c.remove(oldPod)
			c.after(2 * time.Minute)
			message := "spec.storage.size: cannot shrink volume from " +
				"100Gi to 50Gi"
			since := c.now
			for range 6 {
				cr := clusterPostgres(c, 4, "50Gi", 3, "False",
					"ReconcileError", message)
				clusterCondition(cr, "Ready", "False", "ReconcileError",
					message, since)
				c.update(cr)
				c.after(45 * time.Second)
			}
		},
	}
}

// borderlineScheduleAndSecret: the nightly report's credentials Secret
// is rotated, and three minutes later the CronJob's schedule is edited
// to an invalid hour. The CronJob never runs again. The schedule edit
// is the root; the rotation is the plausible alternative.
func borderlineScheduleAndSecret() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-schedule-and-secret",
			Description: "A Secret rotation and an invalid CronJob " +
				"schedule edit land three minutes apart; the CronJob " +
				"cannot parse its schedule.",
			Root: "cronjob/reports/nightly-report", Tier: "notify",
			MaxMessages: 2,
		},
		build: func(c *cluster) {
			secret := configSecret(c, "reports", "report-db", "password")
			cron := scheduleCronJob(c, "reports", "nightly-report",
				"0 2 * * *")
			cron.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env =
				[]corev1.EnvVar{configSecretEnv("DB_PASSWORD",
					secret.Name, "password")}
			c.list(secret, cron)
			c.after(2 * time.Minute)
			rotated := last(c, secret)
			rotated.Data["password"] = []byte("rotated")
			c.update(rotated)
			c.after(3 * time.Minute)
			edited := last(c, cron)
			edited.Spec.Schedule = "0 26 * * *"
			edited.Generation++
			c.update(edited)
			c.after(30 * time.Second)
			c.warn(c.warningEvent(edited, "CronJob", "UnparseableSchedule",
				"unparseable schedule: \"0 26 * * *\" : end of range (26) "+
					"above maximum (23): 26", "cronjob-controller", 1))
		},
	}
}

// borderlineLateConfigReload: the worker's ConfigMap is edited, but the
// pods only pick the file up on their hourly reload, eleven minutes
// later; they then exit with a generic "invalid configuration" that
// names no key. The edit is the root, with weak timing and partial
// evidence.
func borderlineLateConfigReload() scenario {
	return scenario{
		expect: expectation{
			Name: "borderline-late-config-reload",
			Description: "A ConfigMap edit is picked up eleven minutes " +
				"later on a scheduled reload; the pods crash with a " +
				"generic configuration error.",
			Root: "configmap/batch/worker-config", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			cm := configMap(c, "batch", "worker-config",
				map[string]string{"worker.toml": "threads = 8"})
			w := c.deployment("batch", "worker",
				"registry.example.com/worker:12", 2)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: cm.Name,
							},
						},
					},
				}}
			})
			c.list(c.node("n1", "zone-a"), cm)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(time.Minute)
			edited := last(c, cm)
			edited.Data["worker.toml"] = "threads = \"eight\""
			c.update(edited)
			c.after(11 * time.Minute)
			for restarts := int32(1); restarts <= 4; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(78, "Error",
					"fatal: invalid configuration", restarts)),
					w.pod(1, "n1", crashLoop(78, "Error",
						"fatal: invalid configuration", restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(time.Minute)
			}
		},
	}
}
