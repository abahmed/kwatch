package story

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestRolloutStepsWhenCauseIsChange(t *testing.T) {
	depID := knowledge.NewEntityID(kube.KindDeployment, "default",
		"api")

	p := problem.Problem{
		Root: depID,
		Cause: &reason.Hypothesis{
			Change: &knowledge.Change{
				Entity: depID,
			},
		},
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) == 0 {
		t.Error("no rollout steps generated")
	}
	found := false
	for _, step := range steps {
		if strings.Contains(step.Command, "rollout status") {
			found = true
			if step.Mutating {
				t.Errorf("status step should not be mutating")
			}
		}
		if strings.Contains(step.Command, "rollout undo") &&
			!step.Mutating {
			t.Errorf("undo step should be mutating")
		}
	}
	if !found {
		t.Errorf("rollout status command not found in steps")
	}
}

func TestNodeSteps(t *testing.T) {
	nodeID := knowledge.NewEntityID(kube.KindNode, "", "node-1")

	p := problem.Problem{
		Root:    nodeID,
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) == 0 {
		t.Error("no node steps generated")
	}
	found := false
	for _, step := range steps {
		if strings.Contains(step.Command, "describe node") {
			found = true
		}
	}
	if !found {
		t.Errorf("describe node command not found")
	}
}

func TestSecretSteps(t *testing.T) {
	secretID := knowledge.NewEntityID(kube.KindSecret, "default",
		"api-creds")

	p := problem.Problem{
		Root:    secretID,
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) == 0 {
		t.Error("no secret steps generated")
	}
	if !strings.Contains(steps[0].Command, "kubectl get secret") {
		t.Errorf("secret get command not found: %s",
			steps[0].Command)
	}
}

func TestConfigMapSteps(t *testing.T) {
	configID := knowledge.NewEntityID(kube.KindConfigMap, "default",
		"app-config")

	p := problem.Problem{
		Root:    configID,
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) == 0 {
		t.Error("no configmap steps generated")
	}
	if !strings.Contains(steps[0].Command, "kubectl get configmap") {
		t.Errorf("configmap get command not found: %s",
			steps[0].Command)
	}
}

func TestPVCSteps(t *testing.T) {
	pvcID := knowledge.NewEntityID(kube.KindPVC, "default",
		"data-volume")

	p := problem.Problem{
		Root:    pvcID,
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) == 0 {
		t.Error("no pvc steps generated")
	}
	if !strings.Contains(steps[0].Command, "describe pvc") {
		t.Errorf("pvc describe command not found: %s",
			steps[0].Command)
	}
}

func TestContainerLogsSteps(t *testing.T) {
	containerID := knowledge.NewEntityID(kube.KindContainer,
		"default", "api-pod/api-container")

	p := problem.Problem{
		Root: containerID,
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: containerID, Reason: constant.ReasonCrashLoopBackOff}.Key(): {
				Entity:   containerID,
				Reason:   constant.ReasonCrashLoopBackOff,
				Severity: signal.Critical,
				Summary:  "Container is crash looping",
			},
		},
	}

	steps := nextSteps(p, []signal.Signal{
		{
			Entity:   containerID,
			Reason:   constant.ReasonCrashLoopBackOff,
			Severity: signal.Critical,
			Summary:  "Container is crash looping",
		},
	})

	if len(steps) == 0 {
		t.Error("no container log steps generated")
	}
	found := false
	for _, step := range steps {
		if strings.Contains(step.Command, "--previous") {
			found = true
		}
	}
	if !found {
		t.Errorf("logs --previous command not found")
	}
}

func TestOOMKilledStepsIncludeTopCommand(t *testing.T) {
	containerID := knowledge.NewEntityID(kube.KindContainer,
		"default", "app-pod/app")

	p := problem.Problem{
		Root: containerID,
		Members: map[signal.Key]signal.Signal{
			signal.Signal{Entity: containerID, Reason: constant.ReasonOOMKilled}.Key(): {
				Entity:   containerID,
				Reason:   constant.ReasonOOMKilled,
				Severity: signal.Critical,
				Summary:  "Container was OOMKilled",
			},
		},
	}

	steps := nextSteps(p, []signal.Signal{
		{
			Entity:   containerID,
			Reason:   constant.ReasonOOMKilled,
			Severity: signal.Critical,
			Summary:  "Container was OOMKilled",
		},
	})

	foundTop := false
	for _, step := range steps {
		if strings.Contains(step.Command, "top pod") {
			foundTop = true
		}
	}
	if !foundTop {
		t.Errorf("top pod command not found for OOMKilled")
	}
}

func TestStatefulSetRolloutSteps(t *testing.T) {
	ssID := knowledge.NewEntityID(kube.KindStatefulSet, "default",
		"db")

	p := problem.Problem{
		Root: ssID,
		Cause: &reason.Hypothesis{
			Change: &knowledge.Change{Entity: ssID},
		},
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) == 0 {
		t.Error("no statefulset rollout steps")
	}
	cmd := steps[0].Command
	if !strings.Contains(cmd, "statefulset") ||
		!strings.Contains(cmd, "rollout") {
		t.Errorf("statefulset rollout command malformed: %s", cmd)
	}
}

func TestDaemonSetRolloutSteps(t *testing.T) {
	dsID := knowledge.NewEntityID(kube.KindDaemonSet, "default",
		"monitoring")

	p := problem.Problem{
		Root: dsID,
		Cause: &reason.Hypothesis{
			Change: &knowledge.Change{Entity: dsID},
		},
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) == 0 {
		t.Error("no daemonset rollout steps")
	}
	cmd := steps[0].Command
	if !strings.Contains(cmd, "daemonset") ||
		!strings.Contains(cmd, "rollout") {
		t.Errorf("daemonset rollout command malformed: %s", cmd)
	}
}

func TestNoStepsWithoutMembers(t *testing.T) {
	podID := knowledge.NewEntityID(kube.KindPod, "default",
		"test-pod")

	p := problem.Problem{
		Root:    podID,
		Members: map[signal.Key]signal.Signal{},
	}

	steps := nextSteps(p, []signal.Signal{})

	if len(steps) != 0 {
		t.Errorf("expected no steps without members, got %d", len(steps))
	}
}
