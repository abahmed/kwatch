package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// containerScenarios are pods held back by a helper container: an init
// container that keeps failing, a sidecar proxy that keeps crashing.
// The helper is the cause, never the application container.
func containerScenarios() []scenario {
	return []scenario{initContainerFails(), sidecarCrashLoops(),
		livenessKillLoop(), livenessSingleKill()}
}

// initContainerFails: a release adds a migration init container that
// fails on every attempt; the app container never starts. The incident
// belongs to the Deployment, its cause is the init container, and the
// app container is never blamed.
func initContainerFails() scenario {
	return scenario{
		expect: expectation{
			Name: "init-container-fails",
			Description: "An init container crash-loops; the pod's " +
				"application container waits and never starts.",
			Root: "deployment/shop/orders", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "orders",
				"registry.example.com/orders:8", 1)
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.InitContainers = []corev1.Container{{
					Name: "migrate", Image: "registry.example.com/migrate:8",
				}}
			})
			c.list(w.objects())
			c.after(time.Minute)
			message := "ERROR: relation \"orders_v2\" already exists"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", initCrashLoop(message, restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(45 * time.Second)
			}
		},
	}
}

// sidecarCrashLoops: the mesh proxy sidecar of the payments pod crashes
// on a bad bootstrap file; the application container runs fine but the
// pod is never ready. The proxy is the cause.
func sidecarCrashLoops() scenario {
	return scenario{
		expect: expectation{
			Name: "sidecar-crash-loops",
			Description: "A native sidecar proxy crash-loops; the app " +
				"container runs but the pod stays unready.",
			Root: "deployment/shop/payments", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "payments",
				"registry.example.com/payments:5", 1)
			always := corev1.ContainerRestartPolicyAlways
			configTemplate(w, func(spec *corev1.PodSpec) {
				spec.InitContainers = []corev1.Container{{
					Name: "proxy", Image: "registry.example.com/proxy:1.22",
					RestartPolicy: &always,
				}}
			})
			c.list(w.objects())
			c.list(w.pod(0, "n1", sidecarRunning))
			c.after(time.Minute)
			message := "critical: bootstrap: unable to parse " +
				"/etc/proxy/bootstrap.yaml: line 12: mapping values are " +
				"not allowed"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(w.pod(0, "n1", notReady,
					sidecarCrashLoop(message, restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.after(45 * time.Second)
			}
		},
	}
}

// initCrashLoop is a pod whose init container keeps failing: the pod is
// Pending and its app container waits to be initialised.
func initCrashLoop(message string, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		pod.Status.Phase = corev1.PodPending
		pod.Status.InitContainerStatuses = []corev1.ContainerStatus{
			helperCrashed(c, pod.Spec.InitContainers[0], message, restarts),
		}
		app := &pod.Status.ContainerStatuses[0]
		app.Started = boolPtr(false)
		app.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: "PodInitializing"},
		}
	}
}

// sidecarRunning is a healthy pod whose sidecar is up.
func sidecarRunning(c *cluster, pod *corev1.Pod) {
	since := metav1.NewTime(c.start.Add(-time.Hour))
	proxy := pod.Spec.InitContainers[0]
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
		Name: proxy.Name, Image: proxy.Image, Ready: true,
		Started: boolPtr(true),
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{StartedAt: since}},
	}}
}

// sidecarCrashLoop is a pod whose sidecar keeps crashing while the app
// container keeps running.
func sidecarCrashLoop(message string, restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		pod.Status.InitContainerStatuses = []corev1.ContainerStatus{
			helperCrashed(c, pod.Spec.InitContainers[0], message, restarts),
		}
		pod.Status.ContainerStatuses[0].Ready = true
	}
}

// helperCrashed is the status of an init or sidecar container in
// CrashLoopBackOff after its last failed run.
func helperCrashed(
	c *cluster, container corev1.Container, message string,
	restarts int32,
) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: container.Name, Image: container.Image,
		Started: boolPtr(false), RestartCount: restarts,
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  "CrashLoopBackOff",
				Message: "back-off 2m40s restarting failed container",
			},
		},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: 1, Reason: "Error", Message: message,
				StartedAt:  metav1.NewTime(c.now.Add(-20 * time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-15 * time.Second)),
			},
		},
	}
}
