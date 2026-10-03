//go:build e2e

package scenarios

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

const (
	leaderLease     = "kwatch-leader"
	kwatchNamespace = "kwatch"
	receiverWebhook = "http://kwatch-e2e-receiver.kwatch-e2e-system:8080" +
		"/webhook"
)

// lifecycleDeleteLeader deletes the Pod that holds the Lease and waits for
// another Pod to take it. It returns the old and the new holder.
func lifecycleDeleteLeader(s *Scenario) (oldHolder, newHolder string) {
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

// lifecycleExpectIncidents waits until Kwatch has announced count incidents
// for resource, for example after a problem came back.
func lifecycleExpectIncidents(
	s *Scenario, resource, reason string, count int,
) {
	s.T.Helper()
	s.waitForAudit(announceWait(0), harness.AuditMatch{
		Namespace: s.Namespace, Resource: resource,
		Reason: reason, Action: "create", Count: count,
	})
}

// lifecycleReceiverMode makes the webhook receiver answer every request
// with mode ("success" or "http-500") and restores "success" when the test
// ends.
func lifecycleReceiverMode(s *Scenario, mode string) {
	s.T.Helper()
	s.Must(s.Env.Receiver.SetPolicy(s.Ctx, map[string]string{"mode": mode}))
	s.T.Cleanup(func() {
		_ = s.Env.Receiver.SetPolicy(context.Background(),
			map[string]string{"mode": "success"})
	})
}

// lifecycleCreateCrashPod creates a bare Pod that crashes at once.
func lifecycleCreateCrashPod(s *Scenario, name string) {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().Pods(s.Namespace).Create(s.Ctx,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "workload", Image: workloadImage(),
				Command:         []string{"/kwatch-e2e-workload", "crash"},
				ImagePullPolicy: corev1.PullNever,
			}}},
		}, metav1.CreateOptions{})
	s.Must(err)
}

// lifecycleCreateProbeConfig creates a KwatchConfig that probes the
// receiver every second, and deletes it when the test ends.
func lifecycleCreateProbeConfig(s *Scenario) {
	s.T.Helper()
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kwatch.abahmed.dev/v1alpha1",
		"kind":       "KwatchConfig",
		"metadata": map[string]any{
			"name": "kwatch-e2e-active-probe", "namespace": kwatchNamespace,
		},
		"spec": map[string]any{
			"activeProbeMonitor": map[string]any{
				"enabled": true, "intervalSeconds": 1,
				"timeoutSeconds": 1, "failureThreshold": 1,
				"recoveryThreshold": 1,
				"http": []any{map[string]any{
					"name": "receiver", "expectedStatus": 200,
					"url": receiverWebhook,
				}},
			},
		},
	}}
	configs := s.Env.Dynamic.Resource(kwatchConfigResource).
		Namespace(kwatchNamespace)
	_, err := configs.Create(s.Ctx, object, metav1.CreateOptions{})
	s.Must(err)
	s.T.Cleanup(func() {
		_ = configs.Delete(context.Background(), object.GetName(),
			metav1.DeleteOptions{})
	})
}

// lifecycleStartHeartbeatKwatch runs a second Kwatch Pod that sends a
// heartbeat to the receiver every minute, and removes it when the test ends.
func lifecycleStartHeartbeatKwatch(s *Scenario) {
	s.T.Helper()
	base, err := os.ReadFile(filepath.Join(
		"..", "testdata", "base-config.yaml"))
	s.Must(err)
	config := string(base) + "\nheartbeatMonitor:\n  enabled: true\n" +
		"  interval: 1\n  url: \"" + receiverWebhook + "\"\n"
	const name = "kwatch-heartbeat"
	client := s.Env.Client
	_, err = client.CoreV1().Secrets(kwatchNamespace).Create(s.Ctx,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			// base-config.yaml reads the webhook URL from /config/webhook-url.
			Data: map[string][]byte{
				"config.yaml": []byte(config),
				"webhook-url": []byte(receiverWebhook),
			},
		}, metav1.CreateOptions{})
	s.Must(err)
	lifecycleGrantLease(s, name)
	s.T.Cleanup(func() {
		ctx := context.Background()
		opts := metav1.DeleteOptions{}
		if s.T.Failed() {
			lifecycleLogPod(s.T, client, name)
		}
		_ = client.CoreV1().Pods(kwatchNamespace).Delete(ctx, name, opts)
		_ = client.CoordinationV1().Leases(kwatchNamespace).
			Delete(ctx, name, opts)
		_ = client.CoreV1().Secrets(kwatchNamespace).Delete(ctx, name, opts)
	})
	_, err = client.CoreV1().Pods(kwatchNamespace).Create(s.Ctx,
		lifecycleHeartbeatPod(s.Env.Config.KwatchImage, name),
		metav1.CreateOptions{})
	s.Must(err)
}

// lifecycleHeartbeatPod is a restricted Kwatch Pod whose config Secret
// and Lease carry the same name.
func lifecycleHeartbeatPod(image, name string) *corev1.Pod {
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
					AllowPrivilegeEscalation: boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(true),
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

// lifecycleWaitPodRunning waits for a Pod in the Kwatch namespace to run.
func lifecycleWaitPodRunning(s *Scenario, name string) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, 3*time.Minute)
	defer cancel()
	s.Must(s.Env.WaitForPod(ctx, kwatchNamespace, name,
		func(pod *corev1.Pod) bool {
			return pod.Status.Phase == corev1.PodRunning
		}))
}

// lifecycleGrantLease lets the Kwatch ServiceAccount use the extra Lease
// named name. The installed Role only allows the main "kwatch-leader" Lease,
// so a second Kwatch with its own Lease needs this Role and binding. They
// are removed when the test ends.
func lifecycleGrantLease(s *Scenario, name string) {
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

// lifecycleLogPod prints the log of a Pod that is about to be deleted, so a
// failed test explains why the Pod did not work.
func lifecycleLogPod(t *testing.T, client kubernetes.Interface, name string) {
	t.Helper()
	stream, err := client.CoreV1().Pods(kwatchNamespace).
		GetLogs(name, &corev1.PodLogOptions{}).Stream(context.Background())
	if err != nil {
		t.Logf("no log for pod %s: %v", name, err)
		return
	}
	defer stream.Close()
	logs, _ := io.ReadAll(io.LimitReader(stream, 16<<10))
	t.Logf("log of pod %s:\n%s", name, logs)
}

// lifecycleCreateNeedyDeployment creates a Deployment whose Pod reads all of
// its environment from the ConfigMap named configMap, so the Pod cannot
// start until that ConfigMap exists.
func lifecycleCreateNeedyDeployment(s *Scenario, name, configMap string) {
	s.T.Helper()
	labels := map[string]string{"app": name}
	source := corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: configMap},
	}
	_, err := s.Env.Client.AppsV1().Deployments(s.Namespace).Create(s.Ctx,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name:            "workload",
						Image:           workloadImage(),
						Command:         []string{"/kwatch-e2e-workload", "healthy"},
						EnvFrom:         []corev1.EnvFromSource{{ConfigMapRef: &source}},
						ImagePullPolicy: corev1.PullNever,
					}}},
				},
			},
		}, metav1.CreateOptions{})
	s.Must(err)
}

func lifecycleCreateConfigMap(s *Scenario, name string) {
	s.T.Helper()
	_, err := s.Env.Client.CoreV1().ConfigMaps(s.Namespace).Create(s.Ctx,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Data:       map[string]string{"mode": "ok"},
		}, metav1.CreateOptions{})
	s.Must(err)
}

func lifecycleDeleteConfigMap(s *Scenario, name string) {
	s.T.Helper()
	s.Must(s.Env.Client.CoreV1().ConfigMaps(s.Namespace).Delete(s.Ctx, name,
		metav1.DeleteOptions{}))
}

// lifecycleRestartPods replaces the Pods of a Deployment by changing an
// annotation of its Pod template.
func lifecycleRestartPods(s *Scenario, deployment string) {
	s.T.Helper()
	patch := `{"spec":{"template":{"metadata":{"annotations":` +
		`{"kwatch-e2e/restarted":"` +
		time.Now().Format(time.RFC3339Nano) + `"}}}}}`
	_, err := s.Env.Client.AppsV1().Deployments(s.Namespace).Patch(s.Ctx,
		deployment, types.StrategicMergePatchType, []byte(patch),
		metav1.PatchOptions{})
	s.Must(err)
}
