//go:build e2e

package scenarios

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// podsWorkload returns a one-container Pod that runs the e2e workload in
// the given mode ("sleep", "http", "memory", ...).
func podsWorkload(name, mode string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:            "workload",
			Image:           workloadImage(),
			Command:         []string{"/kwatch-e2e-workload", mode},
			ImagePullPolicy: corev1.PullNever,
		}}},
	}
}

// podsCreate creates the pods in the scenario's namespace.
func podsCreate(
	ctx context.Context, e *harness.Environment, namespace string,
	pods ...*corev1.Pod,
) error {
	for _, pod := range pods {
		_, err := e.Client.CoreV1().Pods(namespace).Create(
			ctx, pod, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// podsMissingImage is a Pod whose image is absent and never pulled.
func podsMissingImage(name string) *corev1.Pod {
	pod := podsWorkload(name, "sleep")
	pod.Spec.Containers[0].Image = "example.invalid/kwatch/missing:never"
	return pod
}

// podsFailingInit is a Pod whose init container always fails.
func podsFailingInit(name string) *corev1.Pod {
	pod := podsWorkload(name, "sleep")
	pod.Spec.InitContainers = []corev1.Container{{
		Name:            "init",
		Image:           workloadImage(),
		Command:         []string{"/kwatch-e2e-workload", "startup-error"},
		ImagePullPolicy: corev1.PullNever,
	}}
	return pod
}

// podsDelayedError fails one second after it starts.
func podsDelayedError(name string) *corev1.Pod {
	pod := podsWorkload(name, "delayed-error")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "FAIL_AFTER_SECONDS", Value: "1"},
	}
	return pod
}

// podsNeverReady serves /ready with 503, so its readiness probe fails.
func podsNeverReady(name string) *corev1.Pod {
	pod := podsWorkload(name, "not-ready")
	pod.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/ready", Port: intstr.FromInt(8080),
		}},
	}
	return pod
}

// podsFailingLiveness probes a path that does not exist.
func podsFailingLiveness(name string) *corev1.Pod {
	pod := podsWorkload(name, "http")
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

// podsWithLimit sets one resource limit on the workload container.
func podsWithLimit(
	pod *corev1.Pod, name corev1.ResourceName, quantity string,
) *corev1.Pod {
	pod.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
		name: resource.MustParse(quantity),
	}
	return pod
}

// podsOverMemory allocates 64Mi under a 16Mi limit.
func podsOverMemory(name string) *corev1.Pod {
	pod := podsWorkload(name, "memory")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "MEMORY_MB", Value: "64"},
	}
	return podsWithLimit(pod, corev1.ResourceMemory, "16Mi")
}

// podsTooBig requests far more CPU than any node has.
func podsTooBig(name string) *corev1.Pod {
	pod := podsWorkload(name, "sleep")
	pod.Spec.Containers[0].Resources.Requests = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("1000"),
	}
	return pod
}
