//go:build e2e

package scenarios

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

const stormReplicas = 50

// TestScenarioRootCauseSharedNode stops one node under many Pods and
// expects a single node-rooted page instead of one message per Pod.
func TestScenarioRootCauseSharedNode(t *testing.T) {
	runScenario(t, "rootcause.shared-node", func(
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
		const workloads = 6
		blamed := make([]string, 0, workloads)
		for index := 0; index < workloads; index++ {
			name := fmt.Sprintf("tenant-%d", index)
			blamed = append(blamed, "deployment/"+namespace+"/"+name)
			if err := createDeployment(ctx, e, namespace, name, 1,
				sleepContainer(), pinToNode(node.Name)); err != nil {
				t.Fatal(err)
			}
		}
		runningCtx, runningCancel := context.WithTimeout(ctx, 3*time.Minute)
		defer runningCancel()
		if err := waitForRunningPods(
			runningCtx, e, namespace, workloads,
		); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if err := stopKindNode(ctx, node.Name); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = startKindNode(context.Background(), node.Name) }()
		failureCtx, failureCancel := context.WithTimeout(ctx, 8*time.Minute)
		defer failureCancel()
		assertRoot(failureCtx, t, e, namespace, started,
			harness.RootExpectation{
				Root:             "node//" + node.Name,
				Tier:             "page",
				MaxMessages:      2,
				MaxTotalMessages: 3,
				MustNotBlame:     blamed,
			})
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

// TestScenarioRootCauseSharedRegistry points many workloads at one
// unreachable registry and expects the registry as the single root.
func TestScenarioRootCauseSharedRegistry(t *testing.T) {
	runScenario(t, "rootcause.shared-registry", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		started := time.Now()
		const registry = "registry.kwatch-e2e.invalid"
		var blamed []string
		for index := 0; index < 4; index++ {
			name := fmt.Sprintf("service-%d", index)
			blamed = append(blamed, "deployment/"+namespace+"/"+name)
			if err := createDeployment(ctx, e, namespace, name, 1,
				corev1.Container{
					Name:    "workload",
					Image:   registry + "/team/" + name + ":1",
					Command: []string{"sleep"},
				}, nil); err != nil {
				t.Fatal(err)
			}
		}
		waitCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
		defer cancel()
		if err := e.WaitForPodCount(waitCtx, namespace, func(
			pods []corev1.Pod,
		) bool {
			return len(pods) == len(blamed) && allPodsHaveReason(
				pods, "ErrImagePull", "ImagePullBackOff")
		}); err != nil {
			t.Fatal(err)
		}
		assertRoot(ctx, t, e, namespace, started, harness.RootExpectation{
			Root:             "registry//" + registry,
			Tier:             "notify",
			MaxMessages:      2,
			MaxTotalMessages: 3,
			MustNotBlame:     blamed,
		})
	})
}

// TestScenarioRootCauseSmallStorm crashes 50 Pods of one workload and
// holds the whole namespace to three messages.
func TestScenarioRootCauseSmallStorm(t *testing.T) {
	runScenario(t, "rootcause.small-storm", func(
		ctx context.Context,
		t *testing.T,
		e *harness.Environment,
	) {
		namespace := uniqueNamespace(t.Name())
		if err := createNamespace(ctx, e, namespace); err != nil {
			t.Fatal(err)
		}
		defer cleanupNamespace(t, e, namespace)
		started := time.Now()
		if err := createFailingDeploymentReplicas(
			ctx, e, namespace, "storm", stormReplicas,
		); err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
		defer cancel()
		if err := e.WaitForPodCount(waitCtx, namespace, func(
			pods []corev1.Pod,
		) bool {
			return len(pods) == stormReplicas &&
				allPodsHaveReason(pods, "CrashLoopBackOff")
		}); err != nil {
			t.Fatal(err)
		}
		assertRoot(ctx, t, e, namespace, started, harness.RootExpectation{
			Root:             "deployment/" + namespace + "/storm",
			Tier:             "notify",
			MaxMessages:      3,
			MaxTotalMessages: 3,
			MustNotBlame:     scheduledNodes(ctx, t, e, namespace),
		})
	})
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

func waitForRunningPods(
	ctx context.Context,
	e *harness.Environment,
	namespace string,
	want int,
) error {
	return e.WaitForPodCount(ctx, namespace, func(pods []corev1.Pod) bool {
		running := 0
		for _, pod := range pods {
			if pod.Status.Phase == corev1.PodRunning {
				running++
			}
		}
		return running == want
	})
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
