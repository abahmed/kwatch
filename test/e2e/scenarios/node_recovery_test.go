//go:build e2e

package scenarios

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioNodeRecovery(t *testing.T) {
	runScenario(t, "node.recovery", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		if os.Getenv("KIND_CLUSTER_NAME") == "" {
			t.Skip("KIND_CLUSTER_NAME is required for node scenarios")
		}
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		node, err := firstWorkerNode(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 3; index++ {
			_, err := e.Client.CoreV1().Pods(namespace).Create(ctx,
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(
						"node-impact-%d", index,
					)},
					Spec: corev1.PodSpec{
						NodeName: node.Name,
						Containers: []corev1.Container{{
							Name: "workload", Image: workloadImage(),
							Command:         []string{"/kwatch-e2e-workload", "sleep"},
							ImagePullPolicy: corev1.PullNever,
						}},
					},
				}, metav1.CreateOptions{},
			)
			if err != nil {
				t.Fatal(err)
			}
		}
		baselineCtx, baselineCancel := context.WithTimeout(
			ctx, 3*time.Minute,
		)
		defer baselineCancel()
		if err := e.WaitForPodCount(baselineCtx, namespace, func(
			pods []corev1.Pod,
		) bool {
			if len(pods) != 3 {
				return false
			}
			for _, pod := range pods {
				if pod.Status.Phase != corev1.PodRunning {
					return false
				}
			}
			return true
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.Receiver.Clear(ctx); err != nil {
			t.Fatal(err)
		}
		if err := stopKindNode(ctx, node.Name); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = startKindNode(context.Background(), node.Name) }()
		failureCtx, failureCancel := context.WithTimeout(
			ctx, 7*time.Minute,
		)
		defer failureCancel()
		if err := waitForNodeReady(failureCtx, e, node.Name, false); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Audit.WaitFor(failureCtx, harness.AuditMatch{
			Resource: node.Name, Reason: "NodeNotReady", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		requests, err := e.Receiver.WaitForCount(failureCtx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) != 1 ||
			!strings.Contains(string(requests[0].Body), node.Name) {
			t.Fatalf("shared-node failure was not grouped: %#v", requests)
		}
		if _, err := e.Audit.WaitFor(failureCtx, harness.AuditMatch{
			Resource: node.Name, Reason: "NodeLeaseStale", Count: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if err := waitForKubeletDegradation(failureCtx, e); err != nil {
			t.Fatal(err)
		}
		if err := startKindNode(ctx, node.Name); err != nil {
			t.Fatal(err)
		}
		if err := waitForNodeReady(failureCtx, e, node.Name, true); err != nil {
			t.Fatal(err)
		}
		if err := e.AssertHealthy(ctx); err != nil {
			t.Fatal(err)
		}
	})
}

func waitForKubeletDegradation(
	ctx context.Context,
	e *harness.Environment,
) error {
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond,
		2*time.Minute, true, func(ctx context.Context) (bool, error) {
			body, status, err := e.Health.Get(ctx, "/kubelet", true)
			if err != nil || status < 200 || status >= 300 {
				return false, nil
			}
			degraded := bytes.Contains(body, []byte("partial")) ||
				bytes.Contains(body, []byte("unavailable"))
			return degraded, nil
		})
}

func firstWorkerNode(
	ctx context.Context,
	e *harness.Environment,
) (*corev1.Node, error) {
	nodes, err := e.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for index := range nodes.Items {
		node := &nodes.Items[index]
		if strings.Contains(node.Name, "worker") {
			return node, nil
		}
	}
	return nil, fmt.Errorf("no Kind worker node found")
}

func waitForNodeReady(
	ctx context.Context,
	e *harness.Environment,
	name string,
	wantReady bool,
) error {
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond,
		7*time.Minute, true, func(ctx context.Context) (bool, error) {
			node, err := e.Client.CoreV1().Nodes().Get(
				ctx, name, metav1.GetOptions{},
			)
			if err != nil {
				return false, nil
			}
			for _, condition := range node.Status.Conditions {
				if condition.Type != corev1.NodeReady {
					continue
				}
				return (condition.Status == corev1.ConditionTrue) == wantReady, nil
			}
			return !wantReady, nil
		})
}

func stopKindNode(ctx context.Context, name string) error {
	return runDockerNodeCommand(ctx, "stop", name)
}

func startKindNode(ctx context.Context, name string) error {
	return runDockerNodeCommand(ctx, "start", name)
}

func runDockerNodeCommand(ctx context.Context, action, name string) error {
	output, err := exec.CommandContext(
		ctx, "docker", action, name,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s %s: %w (%s)", action, name, err, output)
	}
	return nil
}
