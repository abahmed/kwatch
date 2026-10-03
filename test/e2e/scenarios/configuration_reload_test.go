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
)

var kwatchConfigResource = schema.GroupVersionResource{
	Group: "kwatch.abahmed.dev", Version: "v1alpha1", Resource: "kwatchconfigs",
}

func TestScenarioConfigurationReload(t *testing.T) {
	onCluster(t, "lifecycle.configuration-reload", func(s *Scenario) {
		cfgCreateLiveConfig(s, "kwatch-e2e-reload",
			map[string]any{"maxRecentLogLines": int64(50)})
		cfgExpectKwatchSurvives(s)
	})
}

func TestScenarioInvalidLiveConfiguration(t *testing.T) {
	onCluster(t, "invalid-config.live-reload", func(s *Scenario) {
		cfgCreateLiveConfig(s, "kwatch-e2e-invalid",
			map[string]any{"workers": int64(-1)})
		cfgExpectKwatchSurvives(s)
	})
}

func TestScenarioInvalidStartupConfiguration(t *testing.T) {
	onCluster(t, "invalid-config.startup", func(s *Scenario) {
		podName := cfgStartKwatchWithBrokenConfig(s)
		cfgWaitForPodFailed(s, podName)
		if containsRuntimePanic(cfgPodLogs(s, podName)) {
			s.T.Fatal("invalid startup configuration caused a runtime panic")
		}
	})
}

func cfgPodLogs(s *Scenario, podName string) string {
	s.T.Helper()
	stream, err := s.Env.Client.CoreV1().Pods("kwatch").GetLogs(
		podName, &corev1.PodLogOptions{},
	).Stream(s.Ctx)
	s.Must(err)
	defer stream.Close()
	payload, err := io.ReadAll(stream)
	s.Must(err)
	return string(payload)
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

// cfgCreateLiveConfig creates a KwatchConfig in the kwatch namespace and
// deletes it when the test ends.
func cfgCreateLiveConfig(s *Scenario, name string, spec map[string]any) {
	s.T.Helper()
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
		"metadata":   map[string]any{"name": name, "namespace": "kwatch"},
		"spec":       spec,
	}}
	configs := s.Env.Dynamic.Resource(kwatchConfigResource).
		Namespace("kwatch")
	_, err := configs.Create(s.Ctx, object, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), slackTime)
		defer cancel()
		_ = configs.Delete(ctx, name, metav1.DeleteOptions{})
	})
}

// cfgExpectKwatchSurvives waits until Kwatch reports healthy and checks it
// did not panic on the new configuration.
func cfgExpectKwatchSurvives(s *Scenario) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, slackTime)
	defer cancel()
	s.Must(wait.PollUntilContextTimeout(
		ctx, 500*time.Millisecond, slackTime, true,
		func(ctx context.Context) (bool, error) {
			return s.Env.AssertHealthy(ctx) == nil, nil
		},
	))
	s.Must(s.Env.AssertNoRuntimePanic(s.Ctx))
}

func cfgWaitForPodFailed(s *Scenario, podName string) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, slackTime)
	defer cancel()
	s.Must(s.Env.WaitForPod(ctx, "kwatch", podName,
		func(pod *corev1.Pod) bool {
			return pod.Status.Phase == corev1.PodFailed
		}))
}

// cfgStartKwatchWithBrokenConfig runs a second Kwatch whose config file is
// not valid YAML and returns its Pod name. Both objects are deleted when
// the test ends.
func cfgStartKwatchWithBrokenConfig(s *Scenario) string {
	s.T.Helper()
	const name = "kwatch-invalid-startup"
	secrets := s.Env.Client.CoreV1().Secrets("kwatch")
	pods := s.Env.Client.CoreV1().Pods("kwatch")
	_, err := secrets.Create(s.Ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       map[string][]byte{"config.yaml": []byte("not: [valid")},
	}, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), slackTime)
		defer cancel()
		_ = pods.Delete(ctx, name, metav1.DeleteOptions{})
		_ = secrets.Delete(ctx, name, metav1.DeleteOptions{})
	})
	_, err = pods.Create(s.Ctx, cfgBrokenConfigPod(s, name),
		metav1.CreateOptions{})
	s.Must(err)
	return name
}

func cfgBrokenConfigPod(s *Scenario, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
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
				Name: "kwatch", Image: s.Env.Config.KwatchImage,
				ImagePullPolicy: corev1.PullNever,
				Env: []corev1.EnvVar{
					{Name: "CONFIG_FILE", Value: "/config/config.yaml"},
					{Name: "POD_NAMESPACE", Value: "kwatch"},
					{Name: "POD_NAME", Value: name},
					{Name: "KWATCH_LEADER_ELECTION_NAME", Value: name},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: boolPtr(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				},
				VolumeMounts: []corev1.VolumeMount{{
					Name: "config", MountPath: "/config",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: name},
				},
			}},
		},
	}
}
