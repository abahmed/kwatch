package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// gap2ReleaseAScenarios are releases whose new pods stay Pending, never
// become ready or exit at once.
func gap2ReleaseAScenarios() []scenario {
	return []scenario{
		gap2SchedulerName(), gap2PVCNotFound(), gap2ProbeExecMissing(),
		gap2ProbeSchemeMismatch(), gap2ScriptNotExecutable(),
		gap2ExitSegfault(), gap2ExitZero(),
	}
}

// gap2NoScheduler is a pod nobody has looked at: Pending, no conditions.
func gap2NoScheduler(c *cluster, pod *corev1.Pod) {
	pod.CreationTimestamp = metav1.NewTime(c.now)
	pod.Status = corev1.PodStatus{
		Phase: corev1.PodPending, QOSClass: corev1.PodQOSBurstable,
	}
}

// gap2SchedulerName: a release sets schedulerName batch-sched, which no
// scheduler in the cluster runs. The default scheduler ignores the pod and
// nothing else picks it up, so it is Pending without one event.
func gap2SchedulerName() scenario {
	return g2Release(g2Exp("release-scheduler-name-not-running",
		"A release sets a schedulerName that no scheduler runs; the new "+
			"pod is Pending with no scheduling events at all.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(15*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.SchedulerName = "batch-sched"
		},
		func(c *cluster, w *workload) {
			pod := w.pod(0, "", gap2NoScheduler)
			pod.Spec.SchedulerName = "batch-sched"
			c.create(pod)
			for range 3 {
				c.after(5 * time.Minute)
				c.update(pod.DeepCopy())
			}
		})
}

// gap2PVCNotFound: a release mounts the claim data-0, which nobody
// created; the scheduler cannot place the pod.
func gap2PVCNotFound() scenario {
	return g2Release(g2Exp("release-pvc-not-found",
		"A release mounts a PersistentVolumeClaim that does not exist; "+
			"the new pod is Pending with FailedScheduling "+
			"\"persistentvolumeclaim not found\".",
		"persistentvolumeclaim/shop/data-0", "notify", "node//n1",
		"node//n2"),
		func(c *cluster, spec *corev1.PodSpec) {
			claim := c.n("data-0")
			spec.Volumes = []corev1.Volume{{Name: "data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.
						PersistentVolumeClaimVolumeSource{ClaimName: claim},
				}}}
		},
		func(c *cluster, w *workload) {
			g2Pending("0/2 nodes are available: persistentvolumeclaim \""+
				c.n("data-0")+"\" not found. preemption: 0/2 nodes are "+
				"available: 2 Preemption is not helpful for scheduling.")(
				c, w)
		})
}

// gap2ProbeExecMissing: a release adds an exec readiness probe that runs
// curl; the distroless image has none. The probe errors every time and
// the new pod runs but never becomes ready.
func gap2ProbeExecMissing() scenario {
	return g2Release(g2Exp("release-probe-exec-binary-missing",
		"A release adds an exec readiness probe that runs curl, which "+
			"the image lacks; the new pod never becomes ready and the "+
			"kubelet reports the probe erroring.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(5*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{
					Command: []string{"curl", "-f", "localhost:8080/ready"},
				}},
				PeriodSeconds: 10,
			}
		},
		g2Unready("Readiness probe errored: rpc error: code = Unknown "+
			"desc = failed to exec in container: failed to start exec "+
			"\"6f1c\": OCI runtime exec failed: exec failed: unable to "+
			"start container process: exec: \"curl\": executable file "+
			"not found in $PATH: unknown"))
}

// gap2ProbeSchemeMismatch: a release switches the readiness probe to
// HTTPS, but the container still serves plain HTTP on that port.
func gap2ProbeSchemeMismatch() scenario {
	return g2Release(g2Exp("release-probe-scheme-mismatch",
		"A release probes readiness over HTTPS on a port that serves "+
			"plain HTTP; the kubelet reports \"server gave HTTP "+
			"response to HTTPS client\" and the new pod never becomes "+
			"ready.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(5*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.
					HTTPGetAction{Path: "/ready", Scheme: corev1.URISchemeHTTPS,
					Port: intstr.FromInt32(8080)}},
				PeriodSeconds: 10,
			}
		},
		g2Unready("Readiness probe failed: Get \"https://10.244.1.9:"+
			"8080/ready\": http: server gave HTTP response to HTTPS "+
			"client"))
}

// gap2ScriptNotExecutable: a release starts the app through a script
// mounted from a ConfigMap with the default mode 0644; the shell cannot
// run it and the container exits 126.
func gap2ScriptNotExecutable() scenario {
	return g2Release(g2Exp("release-script-not-executable",
		"A release starts the app with a ConfigMap-mounted script that "+
			"lacks the execute bit; the container exits 126 with "+
			"\"Permission denied\" at every start.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(c *cluster, spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{configVolume("scripts",
				c.n("start-scripts"))}
			spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
				Name: "scripts", MountPath: "/scripts"}}
			spec.Containers[0].Command = []string{"/bin/sh", "-c",
				"/scripts/start.sh"}
		},
		func(c *cluster, w *workload) {
			c.list(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: c.n("start-scripts"), Namespace: w.deployment.Namespace,
				UID: "uid-start-scripts"},
				Data: map[string]string{"start.sh": "#!/bin/sh\nexec api\n"}})
			g2Crash(126, "Error",
				"/bin/sh: 1: /scripts/start.sh: Permission denied")(c, w)
		})
}

// gap2ExitSegfault: a release moves to a native build of the image that
// crashes with SIGSEGV at start; the container exits 139.
func gap2ExitSegfault() scenario {
	return g2Release(g2Exp("release-exit-139-segfault",
		"A release's new image crashes with a segmentation fault at "+
			"every start (exit code 139) while the old pods serve.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].Image = "registry.example.com/api:2.6-native"
			spec.Containers[0].Resources.Limits = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi")}
		},
		g2Crash(139, "Error", "Segmentation fault (core dumped)"))
}

// gap2ExitZero: a release starts the server as a background daemon, so
// the foreground process exits 0 at once and the pod restarts in a loop.
func gap2ExitZero() scenario {
	return g2Release(g2Exp("release-exit-zero-crash-loop",
		"A release's command forks the server into the background and "+
			"exits 0; the container is restarted and ends in "+
			"CrashLoopBackOff although every exit is a success.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].Command = []string{"/bin/sh", "-c",
				"/app/api --daemon"}
		},
		g2Crash(0, "Completed", ""))
}
