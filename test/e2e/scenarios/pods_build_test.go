//go:build e2e

package scenarios

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// missingImage is never present on a node and is never pulled.
const missingImage = "example.invalid/kwatch/missing:never"

// workloadContainer runs the e2e workload image in one mode: "sleep",
// "healthy", "crash", "http", "memory", "disk", "not-ready", ... The image
// is local only, so a missing image can never be pulled.
func workloadContainer(mode string) corev1.Container {
	return corev1.Container{
		Name:            "workload",
		Image:           workloadImage(),
		Command:         []string{"/kwatch-e2e-workload", mode},
		ImagePullPolicy: corev1.PullNever,
	}
}

// workloadPod is a Pod with one workload container in the given mode.
func workloadPod(name, mode string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{workloadContainer(mode)},
		},
	}
}

// crashingPod crashes as soon as it starts.
func crashingPod(name string) *corev1.Pod {
	return workloadPod(name, "crash")
}

// podWithMissingImage never starts: its image is not on the node.
func podWithMissingImage(name string) *corev1.Pod {
	pod := workloadPod(name, "sleep")
	pod.Spec.Containers[0].Image = missingImage
	return pod
}

// podWithFailingInit has an init container that always fails.
func podWithFailingInit(name string) *corev1.Pod {
	pod := workloadPod(name, "sleep")
	init := workloadContainer("startup-error")
	init.Name = "init"
	pod.Spec.InitContainers = []corev1.Container{init}
	return pod
}

// podWithDelayedError fails one second after it starts.
func podWithDelayedError(name string) *corev1.Pod {
	pod := workloadPod(name, "delayed-error")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "FAIL_AFTER_SECONDS", Value: "1"},
	}
	return pod
}

// podNeverReady serves /ready with 503, so its readiness probe fails.
func podNeverReady(name string) *corev1.Pod {
	pod := workloadPod(name, "not-ready")
	pod.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/ready", Port: intstr.FromInt(8080),
		}},
	}
	return pod
}

// podWithFailingLiveness probes a path that does not exist.
func podWithFailingLiveness(name string) *corev1.Pod {
	pod := workloadPod(name, "http")
	pod.Spec.Containers[0].LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/missing", Port: intstr.FromInt(8080),
		}},
		InitialDelaySeconds: 1,
		PeriodSeconds:       1,
		FailureThreshold:    2,
	}
	return pod
}

// podWithLimit sets one resource limit on the workload container.
func podWithLimit(
	pod *corev1.Pod, name corev1.ResourceName, quantity string,
) *corev1.Pod {
	pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		name: resource.MustParse(quantity),
	}
	return pod
}

// podOverMemoryLimit allocates 64Mi under a 16Mi limit.
func podOverMemoryLimit(name string) *corev1.Pod {
	pod := workloadPod(name, "memory")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "MEMORY_MB", Value: "64"},
	}
	return podWithLimit(pod, corev1.ResourceMemory, "16Mi")
}

// podTooBigToSchedule requests far more CPU than any node has.
func podTooBigToSchedule(name string) *corev1.Pod {
	pod := workloadPod(name, "sleep")
	pod.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("1000"),
	}
	return pod
}

// podWithFailingStartHook runs a postStart hook that exits with an error.
func podWithFailingStartHook(name string) *corev1.Pod {
	pod := workloadPod(name, "sleep")
	pod.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{
		PostStart: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{
			Command: []string{"/kwatch-e2e-workload", "startup-error"},
		}},
	}
	return pod
}

// privilegedPod asks for a privileged container.
func privilegedPod(name string) *corev1.Pod {
	pod := workloadPod(name, "sleep")
	pod.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{
		Privileged: ptr(true),
	}
	return pod
}

// CreatePod creates the Pod in the scenario namespace.
func (s *Scenario) CreatePod(pod *corev1.Pod) {
	s.T.Helper()
	s.Must(s.TryCreatePod(pod))
}

// TryCreatePod is CreatePod for a scenario that expects the API server to
// reject the Pod; it returns the error instead of stopping the test.
func (s *Scenario) TryCreatePod(pod *corev1.Pod) error {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().Pods(s.Namespace).Create(
		s.Ctx, pod, metav1.CreateOptions{})
	return err
}

// WaitForPods waits until the namespace's pods satisfy done.
func (s *Scenario) WaitForPods(
	timeout time.Duration, done func(pods []corev1.Pod) bool,
) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, timeout)
	defer cancel()
	s.Must(s.Env.WaitForPodCount(ctx, s.Namespace, done))
}

// WaitForPodReason waits until the namespace has exactly count pods and
// every one of them shows one of the reasons, such as "CrashLoopBackOff",
// "OOMKilled" or "ErrImagePull".
func (s *Scenario) WaitForPodReason(
	timeout time.Duration, count int, reasons ...string,
) {
	s.T.Helper()
	s.WaitForPods(timeout, func(pods []corev1.Pod) bool {
		return len(pods) == count && allPodsHaveReason(pods, reasons...)
	})
}

// WaitForRunningPods waits until count pods of the namespace run.
func (s *Scenario) WaitForRunningPods(count int) {
	s.T.Helper()
	s.WaitForPods(3*time.Minute, func(pods []corev1.Pod) bool {
		return runningCount(pods) == count
	})
}

func runningCount(pods []corev1.Pod) int {
	running := 0
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning {
			running++
		}
	}
	return running
}

func allPodsHaveReason(pods []corev1.Pod, reasons ...string) bool {
	for index := range pods {
		matched := false
		for _, reason := range reasons {
			if harness.PodHasReason(&pods[index], reason) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
