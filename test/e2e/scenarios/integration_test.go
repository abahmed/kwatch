//go:build e2e

package scenarios

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioActiveProbeFailureAndRecovery(t *testing.T) {
	runScenario(t, "integration.active-probe", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		if err := e.Receiver.SetPolicy(ctx, map[string]string{
			"mode": "http-500",
		}); err != nil {
			t.Fatal(err)
		}
		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kwatch.abahmed.dev/v1alpha1",
			"kind":       "KwatchConfig",
			"metadata": map[string]any{
				"name": "kwatch-e2e-active-probe", "namespace": "kwatch",
			},
			"spec": map[string]any{
				"activeProbeMonitor": map[string]any{
					"enabled": true, "intervalSeconds": 1,
					"timeoutSeconds": 1, "failureThreshold": 1,
					"recoveryThreshold": 1,
					"http": []any{map[string]any{
						"name": "receiver", "expectedStatus": 200,
						"url": "http://kwatch-e2e-receiver." +
							"kwatch-e2e-system.svc.cluster.local:8080/webhook",
					}},
				},
			},
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
			_ = e.Receiver.SetPolicy(context.Background(), map[string]string{
				"mode": "success",
			})
		}()
		waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
			Reason: "ActiveProbeFailure", Action: "create", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.Receiver.SetPolicy(ctx, map[string]string{
			"mode": "success",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(waitCtx, harness.AuditMatch{
			Reason: "ActiveProbeFailure", Action: "resolved", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioHeartbeatDelivery(t *testing.T) {
	runScenario(t, "integration.heartbeat", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		if err := e.Receiver.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		base, err := os.ReadFile(filepath.Join(
			"..", "testdata", "base-config.yaml",
		))
		if err != nil {
			t.Fatal(err)
		}
		config := string(base) + `

heartbeatMonitor:
  enabled: true
  interval: 1
  url: "http://kwatch-e2e-receiver.kwatch-e2e-system:8080/webhook"
`
		secretName := "kwatch-heartbeat"
		podName := "kwatch-heartbeat"
		_, err = e.Client.CoreV1().Secrets("kwatch").Create(ctx,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName},
				Data: map[string][]byte{
					"config.yaml":       []byte(config),
					"diagnostics-token": []byte("e2e-token"),
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = e.Client.CoreV1().Pods("kwatch").Delete(
				context.Background(), podName, metav1.DeleteOptions{},
			)
			_ = e.Client.CoordinationV1().Leases("kwatch").Delete(
				context.Background(), "kwatch-heartbeat", metav1.DeleteOptions{},
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
							Value: "kwatch-heartbeat"},
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
		waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		if err := e.WaitForPod(waitCtx, "kwatch", podName,
			func(pod *corev1.Pod) bool {
				return pod.Status.Phase == corev1.PodRunning
			},
		); err != nil {
			t.Fatal(err)
		}
		var heartbeat bool
		if err := wait.PollUntilContextTimeout(
			waitCtx, 250*time.Millisecond, 2*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				requests, err := e.Receiver.Requests(ctx)
				if err != nil {
					return false, nil
				}
				for _, request := range requests {
					if request.Method == "GET" && len(request.Body) == 0 {
						heartbeat = true
						return true, nil
					}
				}
				return false, nil
			},
		); err != nil {
			t.Fatal(err)
		}
		if !heartbeat {
			t.Fatal("heartbeat request was not delivered")
		}
	})
}
