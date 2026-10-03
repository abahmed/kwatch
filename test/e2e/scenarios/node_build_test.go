//go:build e2e

package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

// infraNamespaces hold the Pods a node scenario must never stop: Kwatch
// (its audit log) and the notification receiver.
var infraNamespaces = []string{"kwatch", "kwatch-e2e-system"}

// RequireKind skips the scenario unless it runs on a Kind cluster, because
// only Kind nodes are Docker containers we can stop.
func (s *Scenario) RequireKind() {
	if os.Getenv("KIND_CLUSTER_NAME") == "" {
		s.T.Skip("KIND_CLUSTER_NAME is required for node scenarios")
	}
}

// FreeWorkerNode picks a worker node that hosts neither Kwatch nor the
// receiver, so stopping it cannot kill the thing that watches or records
// the result. The Kind config has three workers, so one is always free.
func (s *Scenario) FreeWorkerNode() string {
	s.T.Helper()
	busy := map[string]bool{}
	for _, namespace := range infraNamespaces {
		pods, err := s.Env.Client.CoreV1().Pods(namespace).List(
			s.Ctx, metav1.ListOptions{})
		s.Must(err)
		for _, pod := range pods.Items {
			busy[pod.Spec.NodeName] = true
		}
	}
	nodes, err := s.Env.Client.CoreV1().Nodes().List(
		s.Ctx, metav1.ListOptions{})
	s.Must(err)
	for _, node := range nodes.Items {
		if strings.Contains(node.Name, "worker") && !busy[node.Name] {
			return node.Name
		}
	}
	s.T.Fatal("no worker node is free of Kwatch and the receiver")
	return ""
}

// NodeRoots lists every node as a must-not-blame root.
func (s *Scenario) NodeRoots() []string {
	s.T.Helper()
	nodes, err := s.Env.Client.CoreV1().Nodes().List(s.Ctx,
		metav1.ListOptions{})
	s.Must(err)
	roots := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		roots = append(roots, "node//"+node.Name)
	}
	return roots
}

// StopNode stops the node (a Docker container) and waits until Kubernetes
// reports it NotReady. The node is started again when the test ends, even
// if it fails.
func (s *Scenario) StopNode(name string) {
	s.T.Helper()
	s.Must(dockerNode(s.Ctx, "stop", name))
	s.T.Cleanup(func() {
		_ = dockerNode(context.Background(), "start", name)
	})
	s.Must(s.waitForNodeReady(name, false))
}

// StartNode starts a stopped node and waits until it is Ready.
func (s *Scenario) StartNode(name string) {
	s.T.Helper()
	s.Must(dockerNode(s.Ctx, "start", name))
	s.Must(s.waitForNodeReady(name, true))
}

func dockerNode(ctx context.Context, action, name string) error {
	output, err := exec.CommandContext(
		ctx, "docker", action, name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s %s: %w (%s)", action, name, err, output)
	}
	return nil
}

func (s *Scenario) waitForNodeReady(name string, wantReady bool) error {
	nodes := s.Env.Client.CoreV1().Nodes()
	return wait.PollUntilContextTimeout(s.Ctx, 500*time.Millisecond,
		7*time.Minute, true, func(ctx context.Context) (bool, error) {
			node, err := nodes.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			for _, c := range node.Status.Conditions {
				if c.Type == corev1.NodeReady {
					ready := c.Status == corev1.ConditionTrue
					return ready == wantReady, nil
				}
			}
			return !wantReady, nil
		})
}

// MarkNodePressure marks the node as under memory, disk, PID and network
// pressure by patching its status.
func (s *Scenario) MarkNodePressure(name string) {
	s.T.Helper()
	now := metav1.Now()
	var items []map[string]any
	for _, condition := range []corev1.NodeConditionType{
		corev1.NodeMemoryPressure, corev1.NodeDiskPressure,
		corev1.NodePIDPressure, corev1.NodeNetworkUnavailable,
	} {
		items = append(items, map[string]any{
			"type": string(condition), "status": "True",
			"reason": "KwatchE2E", "message": "controlled scenario pressure",
			"lastHeartbeatTime": now, "lastTransitionTime": now,
		})
	}
	patch, err := json.Marshal(map[string]any{
		"status": map[string]any{"conditions": items},
	})
	s.Must(err)
	_, err = s.Env.Client.CoreV1().Nodes().Patch(s.Ctx, name,
		types.StrategicMergePatchType, patch, metav1.PatchOptions{},
		"status")
	s.Must(err)
}

// CreateSleepingPods starts count bare Pods pinned to the node and waits
// until they all run.
func (s *Scenario) CreateSleepingPods(node string, count int) {
	s.T.Helper()
	for index := 0; index < count; index++ {
		pod := workloadPod(fmt.Sprintf("node-impact-%d", index), "sleep")
		pod.Spec.NodeName = node
		s.CreatePod(pod)
	}
	s.WaitForPods(3*time.Minute, func(pods []corev1.Pod) bool {
		return runningCount(pods) == count && len(pods) == count
	})
}

// CreateSleepingDeployments starts count one-replica Deployments pinned to
// the node, waits until their Pods run and returns the roots Kwatch must
// not blame, as "deployment/<ns>/<name>".
func (s *Scenario) CreateSleepingDeployments(
	node string, count int,
) []string {
	s.T.Helper()
	var blamed []string
	for index := 0; index < count; index++ {
		name := fmt.Sprintf("tenant-%d", index)
		blamed = append(blamed, "deployment/"+s.Namespace+"/"+name)
		s.CreateDeployment(name, "sleep", onNode(node))
	}
	s.WaitForRunningPods(count)
	return blamed
}
