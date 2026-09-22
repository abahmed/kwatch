//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodFailureProfiles(t *testing.T) {
	profiles := []struct {
		name    string
		command string
		env     []corev1.EnvVar
	}{
		{name: "startup-error", command: "startup-error"},
		{name: "delayed-error", command: "delayed-error", env: []corev1.EnvVar{
			{Name: "FAIL_AFTER_SECONDS", Value: "1"},
		}},
		{name: "one-shot-error", command: "one-shot-error"},
		{name: "recurring-error", command: "recurring-error"},
	}
	runScenario(t, "pod.failure-profiles", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		for _, profile := range profiles {
			t.Run(profile.name, func(t *testing.T) {
				namespace := uniqueNamespace(t.Name())
				if err := createNamespace(ctx, e, namespace); err != nil {
					t.Fatal(err)
				}
				defer cleanupNamespace(t, e, namespace)
				podName := "failure-profile"
				_, err := e.Client.CoreV1().Pods(namespace).Create(
					ctx, &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: podName},
						Spec: corev1.PodSpec{Containers: []corev1.Container{{
							Name:            "workload",
							Image:           workloadImage(),
							Command:         []string{"/kwatch-e2e-workload", profile.command},
							Env:             profile.env,
							ImagePullPolicy: corev1.PullNever,
						}}},
					}, metav1.CreateOptions{},
				)
				if err != nil {
					t.Fatal(err)
				}
				waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				if err := e.WaitForPod(waitCtx, namespace, podName,
					func(pod *corev1.Pod) bool {
						return harness.PodHasReason(pod, "CrashLoopBackOff")
					},
				); err != nil {
					t.Fatal(err)
				}
				if _, err := e.Audit.WaitFor(ctx, harness.AuditMatch{
					Namespace: namespace,
					Resource:  podName,
					Reason:    "CrashLoopBackOff",
					Count:     1,
				}); err != nil {
					t.Fatal(err)
				}
				if err := e.AssertHealthy(ctx); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
}
