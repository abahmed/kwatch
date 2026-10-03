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

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// stormReplicas is how many Pods the small-storm scenario crashes.
const stormReplicas = 50

// nodeInfraNamespaces hold the Pods a node scenario must never stop:
// Kwatch (its audit log) and the notification receiver.
var nodeInfraNamespaces = []string{"kwatch", "kwatch-e2e-system"}

// nodeRequireKind skips the scenario unless it runs on a Kind cluster,
// because only Kind nodes are Docker containers we can stop.
func nodeRequireKind(s *Scenario) {
	if os.Getenv("KIND_CLUSTER_NAME") == "" {
		s.T.Skip("KIND_CLUSTER_NAME is required for node scenarios")
	}
}

// nodeToStop picks a worker node that hosts neither Kwatch nor the
// receiver, so stopping it cannot kill the thing that watches or records
// the result. The Kind config has three workers, so one is always free.
func nodeToStop(s *Scenario) string {
	s.T.Helper()
	busy := map[string]bool{}
	for _, namespace := range nodeInfraNamespaces {
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

// nodeStopUntilNotReady stops the node (a Docker container) and waits
// until Kubernetes reports it NotReady. The node is started again when the
// test ends, even if it fails.
func nodeStopUntilNotReady(s *Scenario, name string) {
	s.T.Helper()
	s.Must(runDockerNodeCommand(s.Ctx, "stop", name))
	s.T.Cleanup(func() {
		_ = runDockerNodeCommand(context.Background(), "start", name)
	})
	s.Must(waitForNodeReady(s.Ctx, s.Env, name, false))
}

// nodeStartUntilReady starts a stopped node and waits until it is Ready.
func nodeStartUntilReady(s *Scenario, name string) {
	s.T.Helper()
	s.Must(runDockerNodeCommand(s.Ctx, "start", name))
	s.Must(waitForNodeReady(s.Ctx, s.Env, name, true))
}

// nodeCreateSleepingPods starts count bare Pods pinned to the node and
// waits until they all run.
func nodeCreateSleepingPods(s *Scenario, node string, count int) {
	s.T.Helper()
	for index := 0; index < count; index++ {
		_, err := s.Env.Client.CoreV1().Pods(s.Namespace).Create(s.Ctx,
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-impact-%d", index)},
				Spec: corev1.PodSpec{
					NodeName:   node,
					Containers: []corev1.Container{sleepContainer()},
				},
			}, metav1.CreateOptions{})
		s.Must(err)
	}
	s.WaitForPods(3*time.Minute, func(pods []corev1.Pod) bool {
		return runningCount(pods) == count && len(pods) == count
	})
}

// nodeCreateSleepingDeployments starts count one-replica Deployments
// pinned to the node and returns their names as "deployment/<ns>/<name>".
func nodeCreateSleepingDeployments(
	s *Scenario, node string, count int,
) []string {
	s.T.Helper()
	var blamed []string
	for index := 0; index < count; index++ {
		name := fmt.Sprintf("tenant-%d", index)
		blamed = append(blamed, "deployment/"+s.Namespace+"/"+name)
		s.Must(createDeployment(s.Ctx, s.Env, s.Namespace, name, 1,
			sleepContainer(), pinToNode(node)))
	}
	s.WaitForPods(3*time.Minute, func(pods []corev1.Pod) bool {
		return runningCount(pods) == count
	})
	return blamed
}

// nodePressureConditions marks the node as under memory, disk, PID and
// network pressure by patching its status.
func nodePressureConditions(s *Scenario, name string) {
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

func runningCount(pods []corev1.Pod) int {
	running := 0
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning {
			running++
		}
	}
	return running
}

func sleepContainer() corev1.Container {
	return corev1.Container{
		Name: "workload", Image: workloadImage(),
		Command:         []string{"/kwatch-e2e-workload", "sleep"},
		ImagePullPolicy: corev1.PullNever,
	}
}

func pinToNode(name string) func(*corev1.PodSpec) {
	return func(spec *corev1.PodSpec) { spec.NodeName = name }
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

func waitForNodeReady(
	ctx context.Context,
	e *harness.Environment,
	name string,
	wantReady bool,
) error {
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond,
		7*time.Minute, true, func(ctx context.Context) (bool, error) {
			node, err := e.Client.CoreV1().Nodes().Get(
				ctx, name, metav1.GetOptions{})
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

func runDockerNodeCommand(ctx context.Context, action, name string) error {
	output, err := exec.CommandContext(
		ctx, "docker", action, name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s %s: %w (%s)", action, name, err, output)
	}
	return nil
}
