//go:build e2e

package scenarios

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

var kwatchConfigResource = schema.GroupVersionResource{
	Group: "kwatch.abahmed.dev", Version: "v1alpha1", Resource: "kwatchconfigs",
}

func TestScenarioConfigurationReload(t *testing.T) {
	runScenario(t, "lifecycle.configuration-reload", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kwatch.abahmed.dev/v1alpha1",
			"kind":       "KwatchConfig",
			"metadata": map[string]any{
				"name": "kwatch-e2e-reload", "namespace": "kwatch",
			},
			"spec": map[string]any{"maxRecentLogLines": int64(50)},
		}}
		resource := e.Dynamic.Resource(kwatchConfigResource).
			Namespace("kwatch")
		if _, err := resource.Create(
			ctx, object, metav1.CreateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = resource.Delete(context.Background(), object.GetName(),
				metav1.DeleteOptions{})
		}()
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := wait.PollUntilContextTimeout(
			waitCtx, 500*time.Millisecond, 2*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				return e.AssertHealthy(ctx) == nil, nil
			},
		); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertNoRuntimePanic(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioInvalidLiveConfiguration(t *testing.T) {
	runScenario(t, "invalid-config.live-reload", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kwatch.abahmed.dev/v1alpha1",
			"kind":       "KwatchConfig",
			"metadata": map[string]any{
				"name": "kwatch-e2e-invalid", "namespace": "kwatch",
			},
			"spec": map[string]any{"workers": int64(-1)},
		}}
		resource := e.Dynamic.Resource(kwatchConfigResource).
			Namespace("kwatch")
		if _, err := resource.Create(
			ctx, object, metav1.CreateOptions{},
		); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = resource.Delete(context.Background(), object.GetName(),
				metav1.DeleteOptions{})
		}()
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := e.AssertHealthy(waitCtx); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertNoRuntimePanic(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioInvalidStartupConfiguration(t *testing.T) {
	runScenario(t, "invalid-config.startup", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		secretName := "kwatch-invalid-startup"
		podName := "kwatch-invalid-startup"
		_, err := e.Client.CoreV1().Secrets("kwatch").Create(ctx,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName},
				Data: map[string][]byte{
					"config.yaml": []byte("not: [valid"),
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = e.Client.CoreV1().Pods("kwatch").Delete(
				context.Background(), podName, metav1.DeleteOptions{},
			)
			_ = e.Client.CoreV1().Secrets("kwatch").Delete(
				context.Background(), secretName, metav1.DeleteOptions{},
			)
		}()
		_, err = e.Client.CoreV1().Pods("kwatch").Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName},
			Spec: corev1.PodSpec{
				RestartPolicy:      corev1.RestartPolicyNever,
				ServiceAccountName: "kwatch",
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: boolPtr(true),
					RunAsUser:    int64Ptr(1000),
					RunAsGroup:   int64Ptr(1000),
					FSGroup:      int64Ptr(1000),
				},
				Containers: []corev1.Container{{
					Name: "kwatch", Image: e.Config.KwatchImage,
					ImagePullPolicy: corev1.PullNever,
					Env: []corev1.EnvVar{
						{Name: "CONFIG_FILE", Value: "/config/config.yaml"},
						{Name: "POD_NAMESPACE", Value: "kwatch"},
						{Name: "POD_NAME", Value: podName},
						{Name: "KWATCH_LEADER_ELECTION_NAME",
							Value: "kwatch-invalid-startup"},
					},
					VolumeMounts: []corev1.VolumeMount{{
						Name: "config", MountPath: "/config",
					}},
				}},
				Volumes: []corev1.Volume{{
					Name: "config",
					VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
						SecretName: secretName,
					}},
				}},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, "kwatch", podName,
			func(pod *corev1.Pod) bool {
				return pod.Status.Phase == corev1.PodFailed
			},
		); err != nil {
			t.Fatal(err)
		}
		logs, err := readPodLogs(ctx, e, podName)
		if err != nil {
			t.Fatal(err)
		}
		if containsRuntimePanic(logs) {
			t.Fatal("invalid startup configuration caused a runtime panic")
		}
	})
}

func readPodLogs(
	ctx context.Context,
	e *harness.Environment,
	podName string,
) (string, error) {
	stream, err := e.Client.CoreV1().Pods("kwatch").GetLogs(
		podName, &corev1.PodLogOptions{},
	).Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	payload, err := io.ReadAll(stream)
	return string(payload), err
}

func containsRuntimePanic(payload string) bool {
	for _, line := range strings.Fields(payload) {
		line = strings.ToLower(line)
		if strings.Contains(line, "panic") ||
			strings.Contains(line, "invalid memory address") {
			return true
		}
	}
	return false
}

func boolPtr(value bool) *bool {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
