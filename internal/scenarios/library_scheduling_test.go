package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// schedulingScenarios are pods and volumes that cannot be placed or
// bound, and schedules that cannot run.
func schedulingScenarios() []scenario {
	return []scenario{
		schedulerInsufficientMemory(), pvcPendingImmediate(),
		pvcWaitForFirstConsumer(), cronJobInvalidSchedule(),
	}
}

// schedulerInsufficientMemory: a scale-up asks for more memory than any
// worker has free; the cluster-wide memory shortage is the root.
func schedulerInsufficientMemory() scenario {
	return scenario{
		expect: expectation{
			Name: "scheduler-insufficient-memory",
			Description: "A Deployment scales out and its new pods stay " +
				"Pending: 4 workers lack memory and 2 control-plane nodes " +
				"are tainted; preemption cannot help.",
			Root: "scheduling//Insufficient memory", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{
				"node//w1", "scheduling//untolerated taint",
				"scheduler//kube-scheduler",
			},
		},
		build: func(c *cluster) {
			for _, name := range []string{"w1", "w2", "w3", "w4"} {
				c.list(c.node(name, "zone-a"))
			}
			for _, name := range []string{"cp1", "cp2"} {
				c.list(scheduleControlPlane(c, name))
			}
			w := c.deployment("analytics", "etl",
				"registry.example.com/etl:4.1", 2)
			c.list(w.objects())
			c.list(w.pod(0, "w1"), w.pod(1, "w2"))
			c.after(time.Minute)
			setReplicas(w, 5)
			w.setReady(2)
			c.update(w.deployment, w.replicaSet)
			message := "0/6 nodes are available: 4 Insufficient memory, " +
				"2 node(s) had untolerated taint " +
				"{node-role.kubernetes.io/control-plane: }. preemption: " +
				"0/6 nodes are available: 2 Preemption is not helpful " +
				"for scheduling, 4 No preemption victims found for " +
				"incoming pod."
			for i := 2; i < 5; i++ {
				c.create(w.pod(i, "", pendingUnscheduled(message)))
			}
			scheduleFailedEvents(c, w, message, 5)
		},
	}
}

func scheduleControlPlane(c *cluster, name string) *corev1.Node {
	node := c.node(name, "zone-a")
	node.Labels["node-role.kubernetes.io/control-plane"] = ""
	node.Spec.Taints = []corev1.Taint{{
		Key: "node-role.kubernetes.io/control-plane", Effect: "NoSchedule",
	}}
	return node
}

// scheduleFailedEvents repeats the scheduler's FailedScheduling warning
// for the pending replicas every minute for the given minutes.
func scheduleFailedEvents(
	c *cluster, w *workload, message string, minutes int,
) {
	for m := 1; m <= minutes; m++ {
		c.after(time.Minute)
		for i := 2; i < 5; i++ {
			pod := last(c, w.pod(i, ""))
			c.warn(c.warningEvent(pod, "Pod", "FailedScheduling", message,
				"default-scheduler", int32(m)))
		}
	}
}

// pvcPendingImmediate: the claim of a StatefulSet-like database cannot be
// provisioned (the cloud disk quota is used up), so its pod never
// schedules. The claim that cannot bind is the root; the pod is only its
// symptom.
func pvcPendingImmediate() scenario {
	return scenario{
		expect: expectation{
			Name: "pvc-pending-immediate",
			Description: "A claim of an Immediate-binding StorageClass " +
				"stays Pending because provisioning fails; the pod that " +
				"mounts it is unschedulable with unbound claims.",
			Root: "persistentvolumeclaim/data/orders-db", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			c.list(scheduleClass(c, "fast-ssd",
				storagev1.VolumeBindingImmediate))
			claim := scheduleClaim(c, "data", "orders-db", "fast-ssd")
			c.create(claim)
			w := c.deployment("data", "orders-db",
				"registry.example.com/postgres:16", 1)
			w.rollout(func(spec *corev1.PodSpec) {
				spec.Volumes = []corev1.Volume{{
					Name: "data", VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.
							PersistentVolumeClaimVolumeSource{
							ClaimName: claim.Name,
						},
					},
				}}
			})
			w.setReady(0)
			c.create(w.objects())
			message := "0/2 nodes are available: pod has unbound " +
				"immediate PersistentVolumeClaims. preemption: 0/2 nodes " +
				"are available: 2 Preemption is not helpful for " +
				"scheduling."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			provision := "failed to provision volume with StorageClass \"" +
				c.n("fast-ssd") + "\": rpc error: code = ResourceExhausted " +
				"desc = Quota 'SSD_TOTAL_GB' exceeded. Limit: 500.0 in " +
				"region europe-west1."
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				c.warn(c.warningEvent(claim, "PersistentVolumeClaim",
					"ProvisioningFailed", provision,
					"pd.csi.storage.gke.io", m))
			}
		},
	}
}

func scheduleClass(
	c *cluster, name string, mode storagev1.VolumeBindingMode,
) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta:        c.meta("", name),
		Provisioner:       "pd.csi.storage.gke.io",
		VolumeBindingMode: &mode,
	}
}

// scheduleClaim is a Pending 20Gi claim of class.
func scheduleClaim(
	c *cluster, namespace, name, class string,
) *corev1.PersistentVolumeClaim {
	className := c.n(class)
	meta := c.meta(namespace, name)
	meta.CreationTimestamp = metav1.NewTime(c.now)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: meta,
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: &className,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("20Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}
}

// pvcWaitForFirstConsumer: a claim of a WaitForFirstConsumer class is
// Pending by design until a pod uses it. Nothing is wrong.
func pvcWaitForFirstConsumer() scenario {
	return scenario{
		expect: expectation{
			Name: "pvc-wffc-no-consumer",
			Description: "A claim of a WaitForFirstConsumer StorageClass " +
				"with no pod using it stays Pending for 30 minutes, as " +
				"designed; the scheduler reports WaitForFirstConsumer.",
			Quiet: true, Tail: duration(45 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			c.list(scheduleClass(c, "standard-rwo",
				storagev1.VolumeBindingWaitForFirstConsumer))
			claim := scheduleClaim(c, "reports", "archive", "standard-rwo")
			c.create(claim)
			for m := 5; m <= 30; m += 5 {
				c.after(5 * time.Minute)
				ev := c.warningEvent(claim, "PersistentVolumeClaim",
					"WaitForFirstConsumer", "waiting for first consumer "+
						"to be created before binding",
					"persistentvolume-controller", int32(m))
				ev.Type = corev1.EventTypeNormal
				c.warn(ev)
			}
		},
	}
}

// cronJobInvalidSchedule: a nightly report CronJob was given an hour of
// 25; it will never run. The CronJob itself is the root.
func cronJobInvalidSchedule() scenario {
	return scenario{
		expect: expectation{
			Name: "cronjob-invalid-schedule",
			Description: "A CronJob's schedule is edited to \"0 25 * * *\", " +
				"which is not a valid cron expression, so it never runs.",
			Root: "cronjob/reports/nightly-report", Tier: "notify",
			MaxMessages: 1,
		},
		build: func(c *cluster) {
			cron := scheduleCronJob(c, "reports", "nightly-report",
				"0 2 * * *")
			c.list(cron)
			c.after(2 * time.Minute)
			edited := last(c, cron)
			edited.Spec.Schedule = "0 25 * * *"
			edited.Generation++
			c.update(edited)
			c.after(30 * time.Second)
			c.warn(c.warningEvent(edited, "CronJob", "UnparseableSchedule",
				"unparseable schedule: \"0 25 * * *\" : end of range (25) "+
					"above maximum (23): 25", "cronjob-controller", 1))
		},
	}
}

func scheduleCronJob(
	c *cluster, namespace, name, schedule string,
) *batchv1.CronJob {
	last := metav1.NewTime(c.start.Add(-8 * time.Hour))
	cron := &batchv1.CronJob{
		ObjectMeta: c.meta(namespace, name),
		Spec: batchv1.CronJobSpec{
			Schedule: schedule,
			JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{{
						Name: "report", Image: "registry.example.com/report:3",
					}},
				}},
			}},
		},
		Status: batchv1.CronJobStatus{
			LastScheduleTime: &last, LastSuccessfulTime: &last,
		},
	}
	cron.Generation = 1
	return cron
}
