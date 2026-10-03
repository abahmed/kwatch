//go:build e2e

package scenarios

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

const (
	leaderLease     = "kwatch-leader"
	kwatchNamespace = "kwatch"
	receiverWebhook = "http://kwatch-e2e-receiver.kwatch-e2e-system:8080" +
		"/webhook"
)

var kwatchConfigResource = schema.GroupVersionResource{
	Group: "kwatch.abahmed.dev", Version: "v1alpha1", Resource: "kwatchconfigs",
}

// DeleteLeader deletes the Pod that holds the Lease and waits for another
// Pod to take it. It returns the old and the new holder.
func (s *Scenario) DeleteLeader() (oldHolder, newHolder string) {
	s.T.Helper()
	oldHolder, err := s.Env.WaitForLeaseHolder(
		s.Ctx, kwatchNamespace, leaderLease)
	s.Must(err)
	s.Must(s.Env.Client.CoreV1().Pods(kwatchNamespace).Delete(
		s.Ctx, oldHolder, metav1.DeleteOptions{}))
	ctx, cancel := context.WithTimeout(s.Ctx, 5*time.Minute)
	defer cancel()
	newHolder, err = s.Env.WaitForLeaseChange(
		ctx, kwatchNamespace, leaderLease, oldHolder)
	s.Must(err)
	return oldHolder, newHolder
}

// ExpectKwatchAvailable checks that the Kwatch Pod reports itself available.
func (s *Scenario) ExpectKwatchAvailable() {
	s.T.Helper()
	s.Must(s.Env.Health.AssertOK(s.Ctx, "/availabilityz"))
}

// ClearReceiver forgets every notification the receiver has stored.
func (s *Scenario) ClearReceiver() {
	s.T.Helper()
	s.Must(s.Env.Receiver.Clear(s.Ctx))
}

// SetReceiverMode makes the webhook receiver answer every request with mode
// ("success" or "http-500") and restores "success" when the test ends.
func (s *Scenario) SetReceiverMode(mode string) {
	s.T.Helper()
	s.Must(s.Env.Receiver.SetPolicy(s.Ctx, map[string]string{"mode": mode}))
	s.T.Cleanup(func() {
		_ = s.Env.Receiver.SetPolicy(context.Background(),
			map[string]string{"mode": "success"})
	})
}

// WaitForDeliveries waits until the receiver holds count notifications that
// match.
func (s *Scenario) WaitForDeliveries(match harness.DeliveryMatch, count int) {
	s.T.Helper()
	_, err := s.Env.Receiver.WaitForMatchCount(s.Ctx, match, count)
	s.Must(err)
}

// ExpectDeliveries checks that the receiver holds exactly count
// notifications that match.
func (s *Scenario) ExpectDeliveries(match harness.DeliveryMatch, count int) {
	s.T.Helper()
	deliveries, err := s.Env.Receiver.Matching(s.Ctx, match)
	s.Must(err)
	if len(deliveries) != count {
		s.T.Fatalf("receiver holds %d matching deliveries, want %d",
			len(deliveries), count)
	}
}

// ExpectDeliveryRejected waits for the first request the receiver gets and
// checks that it answered with a server error.
func (s *Scenario) ExpectDeliveryRejected() {
	s.T.Helper()
	requests, err := s.Env.Receiver.WaitForCount(s.Ctx, 1)
	s.Must(err)
	if requests[0].Status < 500 {
		s.T.Fatalf("expected provider failure, got HTTP %d",
			requests[0].Status)
	}
}

// ExpectHeartbeat waits until the receiver gets a bodyless GET, which is
// what a Kwatch heartbeat sends.
func (s *Scenario) ExpectHeartbeat() {
	s.T.Helper()
	s.Must(wait.PollUntilContextTimeout(s.Ctx,
		250*time.Millisecond, 2*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			requests, _ := s.Env.Receiver.Requests(ctx)
			for _, request := range requests {
				if request.Method == "GET" && len(request.Body) == 0 {
					return true, nil
				}
			}
			return false, nil
		}))
}

// activeProbeSpec probes the receiver every second.
func activeProbeSpec() map[string]any {
	return map[string]any{
		"activeProbeMonitor": map[string]any{
			"enabled": true, "intervalSeconds": 1,
			"timeoutSeconds": 1, "failureThreshold": 1,
			"recoveryThreshold": 1,
			"http": []any{map[string]any{
				"name": "receiver", "expectedStatus": 200,
				"url": receiverWebhook,
			}},
		},
	}
}

// CreateKwatchConfig creates a KwatchConfig in the Kwatch namespace and
// deletes it when the test ends. Kwatch restarts to apply a valid one.
func (s *Scenario) CreateKwatchConfig(name string, spec map[string]any) {
	s.T.Helper()
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
		"metadata": map[string]any{
			"name": name, "namespace": kwatchNamespace,
		},
		"spec": spec,
	}}
	configs := s.Env.Dynamic.Resource(kwatchConfigResource).
		Namespace(kwatchNamespace)
	_, err := configs.Create(s.Ctx, object, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), slackTime)
		defer cancel()
		_ = configs.Delete(ctx, name, metav1.DeleteOptions{})
	})
}

// StartHeartbeatKwatch runs a second Kwatch Pod that sends a heartbeat to
// the receiver every minute and waits until it runs. The Pod is removed
// when the test ends.
func (s *Scenario) StartHeartbeatKwatch() {
	s.T.Helper()
	base, err := os.ReadFile(filepath.Join(
		"..", "testdata", "base-config.yaml"))
	s.Must(err)
	// The heartbeat URL is sensitive, so Kwatch only accepts it as a file
	// reference to a mounted Secret key.
	config := string(base) + "\nheartbeatMonitor:\n  enabled: true\n" +
		"  interval: 1\n  url: \"${file:/config/heartbeat-url}\"\n"
	const name = "kwatch-heartbeat"
	s.GrantLease(name)
	s.StartKwatch(name, map[string][]byte{
		"config.yaml": []byte(config),
		// base-config.yaml reads the webhook URL from /config/webhook-url.
		"webhook-url":   []byte(receiverWebhook),
		"heartbeat-url": []byte(receiverWebhook),
	})
	s.WaitForKwatchPod(name, corev1.PodRunning)
}

// StartKwatchWithBrokenConfig runs a second Kwatch whose config file is not
// valid YAML and returns its Pod name.
func (s *Scenario) StartKwatchWithBrokenConfig() string {
	s.T.Helper()
	const name = "kwatch-invalid-startup"
	s.StartKwatch(name, map[string][]byte{
		"config.yaml": []byte("not: [valid"),
	})
	return name
}

// StartKwatch creates a Secret with the given config files and a second
// Kwatch Pod that mounts it as /config. Both are deleted when the test
// ends; a failed test prints the Pod's log first.
func (s *Scenario) StartKwatch(name string, files map[string][]byte) {
	s.T.Helper()
	client := s.Env.Client
	_, err := client.CoreV1().Secrets(kwatchNamespace).Create(s.Ctx,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Data:       files,
		}, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx := context.Background()
		opts := metav1.DeleteOptions{}
		if s.T.Failed() {
			logKwatchPod(s.T, s.Env, name)
		}
		_ = client.CoreV1().Pods(kwatchNamespace).Delete(ctx, name, opts)
		_ = client.CoordinationV1().Leases(kwatchNamespace).
			Delete(ctx, name, opts)
		_ = client.CoreV1().Secrets(kwatchNamespace).Delete(ctx, name, opts)
	})
	_, err = client.CoreV1().Pods(kwatchNamespace).Create(s.Ctx,
		kwatchPod(s.Env.Config.KwatchImage, name), metav1.CreateOptions{})
	s.Must(err)
}

// kwatchPod is a restricted Kwatch Pod whose config Secret and Lease carry
// the same name.
func kwatchPod(image, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: "kwatch",
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr(true),
				RunAsUser:    ptr(int64(1000)),
				RunAsGroup:   ptr(int64(1000)),
				FSGroup:      ptr(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{{
				Name: "kwatch", Image: image,
				ImagePullPolicy: corev1.PullNever,
				Env: []corev1.EnvVar{
					{Name: "CONFIG_FILE", Value: "/config/config.yaml"},
					{Name: "POD_NAMESPACE", Value: kwatchNamespace},
					{Name: "POD_NAME", Value: name},
					{Name: "KWATCH_LEADER_ELECTION_NAME", Value: name},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr(false),
					ReadOnlyRootFilesystem:   ptr(true),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "config", MountPath: "/config"},
					{Name: "data", MountPath: "/var/lib/kwatch"},
				},
			}},
			Volumes: []corev1.Volume{
				{Name: "config", VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: name}}},
				{Name: "data", VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}
}

// WaitForKwatchPod waits for a Pod in the Kwatch namespace to reach phase.
func (s *Scenario) WaitForKwatchPod(name string, phase corev1.PodPhase) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, 3*time.Minute)
	defer cancel()
	s.Must(s.Env.WaitForPod(ctx, kwatchNamespace, name,
		func(pod *corev1.Pod) bool { return pod.Status.Phase == phase }))
}

// ExpectNoPanicInLog checks that the log of a Kwatch Pod has no runtime
// panic.
func (s *Scenario) ExpectNoPanicInLog(name string) {
	s.T.Helper()
	stream, err := s.Env.Client.CoreV1().Pods(kwatchNamespace).GetLogs(
		name, &corev1.PodLogOptions{}).Stream(s.Ctx)
	s.Must(err)
	defer stream.Close()
	payload, err := io.ReadAll(stream)
	s.Must(err)
	if containsRuntimePanic(string(payload)) {
		s.T.Fatal("invalid startup configuration caused a runtime panic")
	}
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

// logKwatchPod prints the log of a Pod that is about to be deleted, so a
// failed test explains why the Pod did not work.
func logKwatchPod(t *testing.T, e *harness.Environment, name string) {
	t.Helper()
	stream, err := e.Client.CoreV1().Pods(kwatchNamespace).
		GetLogs(name, &corev1.PodLogOptions{}).Stream(context.Background())
	if err != nil {
		t.Logf("no log for pod %s: %v", name, err)
		return
	}
	defer stream.Close()
	logs, _ := io.ReadAll(io.LimitReader(stream, 16<<10))
	t.Logf("log of pod %s:\n%s", name, logs)
}

// GrantLease lets the Kwatch ServiceAccount use the extra Lease named name.
// The installed Role only allows the main "kwatch-leader" Lease, so a second
// Kwatch with its own Lease needs this Role and binding. They are removed
// when the test ends.
func (s *Scenario) GrantLease(name string) {
	s.T.Helper()
	client := s.Env.Client.RbacV1()
	_, err := client.Roles(kwatchNamespace).Create(s.Ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules: []rbacv1.PolicyRule{{
			APIGroups:     []string{"coordination.k8s.io"},
			Resources:     []string{"leases"},
			ResourceNames: []string{name},
			Verbs:         []string{"get", "update"},
		}},
	}, metav1.CreateOptions{})
	s.Must(err)
	_, err = client.RoleBindings(kwatchNamespace).Create(s.Ctx,
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role", Name: name,
			},
			Subjects: []rbacv1.Subject{{
				Kind: "ServiceAccount", Name: "kwatch",
				Namespace: kwatchNamespace,
			}},
		}, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		ctx := context.Background()
		opts := metav1.DeleteOptions{}
		_ = client.RoleBindings(kwatchNamespace).Delete(ctx, name, opts)
		_ = client.Roles(kwatchNamespace).Delete(ctx, name, opts)
	})
}
