package scenarios

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// gap4aCtrlScenarios are devices, resizes, sidecars and probes of newer
// features, and jobs and pods whose end must stay quiet.
func gap4aCtrlScenarios() []scenario {
	return []scenario{
		gap4aDRADriverDown(), gap4aResizeError(), gap4aSwapNotices(),
		gap4aPodLevelOOM(), gap4aSidecarStartup(),
		gap4aSuccessPolicy(), gap4aReplacementStuck(),
		gap4aJobSuspended(), gap4aEphemeralFails(),
	}
}

// gap4aDRADriverDown: the DRA kubelet plugin of n2 crash-loops; the
// trainer pod restarted there cannot prepare its allocated claim.
func gap4aDRADriverDown() scenario {
	return scenario{
		expect: g2Exp("dra-driver-down-prepare-fails",
			"A device driver's kubelet plugin crash-loops on a node; a "+
				"pod with an allocated claim there stays in "+
				"ContainerCreating with FailedPrepareDynamicResources.",
			"daemonset/kube-system/gpu-dra-driver", "notify", "node//n2",
			"deployment/ml/trainer").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			ds := g2Daemon(c, "kube-system", "gpu-dra-driver", "n1", "n2")
			w := c.deployment("ml", "trainer", "registry.example.com/t:3", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n2"))
			c.after(10 * time.Minute)
			crash := "failed to serve: listen unix /var/lib/kubelet/" +
				"plugins/gpu.example.com/dra.sock: bind: address " +
				"already in use"
			message := "Failed to prepare dynamic resources: prepare " +
				"dynamic resources: NodePrepareResources failed for claim " +
				"ml/gpu-claim: rpc error: code = Unavailable desc = " +
				"connection error: desc = \"transport: Error while " +
				"dialing: dial unix /var/lib/kubelet/plugins/gpu." +
				"example.com/dra.sock: connect: no such file or directory\""
			c.remove(w.pod(0, "n2"))
			created := c.now
			for round := int32(1); round <= 6; round++ {
				setAgentCounts(ds, 2, 1)
				c.update(ds, agentPod(c, ds, "n2", crashLoop(1, "Error",
					crash, round)))
				pod := w.pod(1, "n2", createdAt(created),
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				gapKubelet(c, pod, "FailedPrepareDynamicResources", message,
					round)
				c.after(time.Minute)
			}
		},
	}
}

// gap4aResizeError: an autoscaler lowers a pod's memory below its usage;
// the kubelet accepts the resize and cannot complete it.
func gap4aResizeError() scenario {
	return scenario{
		expect: g2Exp("pod-resize-error-in-progress",
			"A pod's in-place memory resize stays in progress and the "+
				"kubelet reports ResizeError because usage is above the "+
				"new limit.",
			"deployment/shop/api", "digest", "node//n1").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			message := "memory limit 256Mi is below the current usage " +
				"of the container"
			for m := int32(1); m <= 5; m++ {
				pod := w.pod(0, "n1")
				pod.Status.Conditions = append(pod.Status.Conditions,
					corev1.PodCondition{Type: "PodResizeInProgress",
						Status: corev1.ConditionTrue, Reason: "Error",
						Message: message})
				c.update(pod)
				gapKubelet(c, pod, "ResizeError", message, m)
				c.after(3 * time.Minute)
			}
		},
	}
}

// gap4aSwapNotices: nodes with swap warn about limits not enforced and
// memory-backed volumes on disk for healthy pods.
func gap4aSwapNotices() scenario {
	return scenario{
		expect: g2Quiet("node-swap-notices-healthy-pods-quiet",
			"Pods on swap-enabled nodes get SwapLimitNotEnforced and "+
				"PossibleMemoryBackedVolumesOnDisk warnings and are "+
				"ready."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 3)
			c.list(w.objects())
			for i := range 3 {
				pod := w.pod(i, "n1")
				c.list(pod)
				gapKubelet(c, pod, "SwapLimitNotEnforced", "Swap limit "+
					"is not enforced for the container", 1)
				gapKubelet(c, pod, "PossibleMemoryBackedVolumesOnDisk",
					"The tmpfs noswap option is not supported; memory-"+
						"backed volumes may be written to disk", 1)
			}
			c.after(10 * time.Minute)
		},
	}
}

// gap4aPodLevelOOM: a release sets a pod-level memory limit of 512Mi that
// the two containers together exceed; the kernel kills one container
// that is below its own request.
func gap4aPodLevelOOM() scenario {
	return g2Release(g2Exp("release-pod-level-memory-limit-oom",
		"A release sets a pod-level memory limit that its containers "+
			"together exceed; one container is OOMKilled repeatedly.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Resources = &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.
					MustParse("512Mi")}}
		},
		g2Crash(137, "OOMKilled", ""))
}

// gap4aSidecarStartup: a release adds a native sidecar whose startup probe
// targets the wrong port; the main container never starts.
func gap4aSidecarStartup() scenario {
	return g2Release(g2Exp("release-sidecar-startup-probe-fails",
		"A release adds a restartable init container (a native "+
			"sidecar) whose startup probe checks the wrong port; the "+
			"pod stays in PodInitializing and the main container never "+
			"starts.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			always := corev1.ContainerRestartPolicyAlways
			spec.InitContainers = []corev1.Container{{Name: "proxy",
				Image:         "registry.example.com/proxy:1.4",
				RestartPolicy: &always,
				StartupProbe: &corev1.Probe{ProbeHandler: corev1.
					ProbeHandler{TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt32(15021)}}}}}
		},
		func(c *cluster, w *workload) {
			gapRounds(c, w, 8, 30*time.Second, func(int32) podState {
				return gapPending(waiting("PodInitializing", ""))
			}, func(pod *corev1.Pod, round int32) {
				gapKubelet(c, pod, "Unhealthy", "Startup probe failed: dial "+
					"tcp 10.244.1.9:15021: connect: connection refused",
					round)
			})
		})
}

// gap4aSuccessPolicy: an indexed Job with a success policy completes when
// index 0 succeeds; the other pods are terminated.
func gap4aSuccessPolicy() scenario {
	return scenario{
		expect: g2Quiet("job-success-policy-terminates-other-pods-quiet",
			"An indexed Job completes through its successPolicy when "+
				"index 0 succeeds; its other pods are terminated "+
				"with exit code 143."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			job := retryJob(c, "mpi-train", 0)
			c.create(job)
			c.after(5 * time.Minute)
			for i := range 4 {
				pod := readyJobPod(c, retryPod(c, job, i, ""))
				pod.Spec.NodeName = c.n("n1")
				c.create(pod)
			}
			c.after(5 * time.Minute)
			done := job.DeepCopy()
			done.Status.Active, done.Status.Succeeded = 0, 1
			done.Status.Conditions = []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
				Reason:             "SuccessPolicy",
				Message:            "Matched rules at index 0",
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(done)
			for i := 1; i < 4; i++ {
				pod := readyJobPod(c, retryPod(c, job, i,
					"terminated by the Job"))
				pod.Spec.NodeName = c.n("n1")
				pod.Status.ContainerStatuses[0].State.Terminated.ExitCode = 143
				c.update(pod)
			}
			c.after(10 * time.Minute)
		},
	}
}

// gap4aReplacementStuck: a Job waits for failed pods before replacing
// them; its pod is Terminating on a node that stopped reporting, so no
// replacement is ever created.
func gap4aReplacementStuck() scenario {
	return scenario{
		expect: g2Exp("job-replacement-waits-for-terminating-pod",
			"A Job with podReplacementPolicy Failed has no replacement "+
				"pod because its pod is Terminating on a node that "+
				"stopped reporting.",
			"node//n1", "notify", "job/batch/nightly-export").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"))
			job := retryJob(c, "nightly-export", 3)
			policy := batchv1.Failed
			job.Spec.PodReplacementPolicy = &policy
			c.create(job)
			pod := readyJobPod(c, retryPod(c, job, 0, ""))
			pod.Spec.NodeName = c.n("n1")
			c.create(pod)
			c.after(10 * time.Minute)
			nodeLose(c, n1)
			c.after(40 * time.Second)
			stuck := retryPod(c, job, 0, "")
			stuck.Spec.NodeName = c.n("n1")
			notReady(c, stuck)
			deleting(&stuck.ObjectMeta, c.now)
			c.update(stuck)
			for range 4 {
				c.after(5 * time.Minute)
				c.update(stuck.DeepCopy())
			}
		},
	}
}

// gap4aJobSuspended: an operator suspends a running Job; its pods are
// deleted and the Job waits.
func gap4aJobSuspended() scenario {
	return scenario{
		expect: g2Quiet("job-suspended-while-running-quiet",
			"An operator sets suspend on a running Job; the controller "+
				"deletes its pods and the Job reads Suspended."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			job := retryJob(c, "reindex", 3)
			c.create(job)
			pod := readyJobPod(c, retryPod(c, job, 0, ""))
			pod.Spec.NodeName = c.n("n1")
			c.create(pod)
			c.after(10 * time.Minute)
			suspended := job.DeepCopy()
			yes := true
			suspended.Spec.Suspend = &yes
			suspended.Status.Active = 0
			suspended.Status.Conditions = []batchv1.JobCondition{{
				Type: batchv1.JobSuspended, Status: corev1.ConditionTrue,
				Reason: "JobSuspended", Message: "Job suspended",
				LastTransitionTime: metav1.NewTime(c.now)}}
			c.update(suspended)
			c.remove(pod)
			c.after(15 * time.Minute)
		},
	}
}

// gap4aEphemeralFails: an engineer attaches a debug container whose image
// cannot be pulled; the pod itself stays ready.
func gap4aEphemeralFails() scenario {
	return scenario{
		expect: g2Quiet("ephemeral-debug-container-fails-quiet",
			"A debug container attached to a healthy pod cannot pull "+
				"its image; the pod's own containers stay ready."),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(10 * time.Minute)
			pod := w.pod(0, "n1")
			pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{{
				EphemeralContainerCommon: corev1.EphemeralContainerCommon{
					Name: "debugger-x7k2p", Image: "registry.example.com/" +
						"tools/netshoot:latest"}}}
			pod.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{{
				Name: "debugger-x7k2p", State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff", Message: "Back-off " +
							"pulling image \"registry.example.com/tools/" +
							"netshoot:latest\""}}}}
			c.update(pod)
			gapKubelet(c, pod, "Failed", "Failed to pull image \"registry."+
				"example.com/tools/netshoot:latest\": not found", 3)
			c.after(15 * time.Minute)
		},
	}
}
