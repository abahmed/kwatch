package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestRolloutStepsWhenCauseIsChange(t *testing.T) {
	depID := inventory.CoreID(kube.KindDeployment, "default",
		"api")

	p := incident.Incident{
		Root: depID,
		Cause: &rootcause.CauseRecord{
			Change: &inventory.Change{
				Entity: depID,
			},
		},
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

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
	nodeID := inventory.CoreID(kube.KindNode, "", "node-1")

	p := incident.Incident{
		Root:    nodeID,
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

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
	secretID := inventory.CoreID(kube.KindSecret, "default",
		"api-creds")

	p := incident.Incident{
		Root:    secretID,
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

	if len(steps) == 0 {
		t.Error("no secret steps generated")
	}
	if !strings.Contains(steps[0].Command, "kubectl describe secret") {
		t.Errorf("secret describe command not found: %s",
			steps[0].Command)
	}
}

func TestConfigMapSteps(t *testing.T) {
	configID := inventory.CoreID(kube.KindConfigMap, "default",
		"app-config")

	p := incident.Incident{
		Root:    configID,
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

	if len(steps) == 0 {
		t.Error("no configmap steps generated")
	}
	if !strings.Contains(steps[0].Command, "kubectl get configmap") {
		t.Errorf("configmap get command not found: %s",
			steps[0].Command)
	}
}

func TestPVCSteps(t *testing.T) {
	pvcID := inventory.CoreID(kube.KindPVC, "default",
		"data-volume")

	p := incident.Incident{
		Root:    pvcID,
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

	if len(steps) == 0 {
		t.Error("no pvc steps generated")
	}
	if !strings.Contains(steps[0].Command, "describe pvc") {
		t.Errorf("pvc describe command not found: %s",
			steps[0].Command)
	}
}

func TestContainerLogsSteps(t *testing.T) {
	containerID := inventory.CoreID(kube.KindContainer,
		"default", "api-pod/api-container")

	p := incident.Incident{
		Root: containerID,
		Members: map[detection.Key]detection.Finding{
			detection.Finding{
				Entity: containerID, Reason: reasons.CrashLoopBackOff,
			}.Key(): {
				Entity:   containerID,
				Reason:   reasons.CrashLoopBackOff,
				Severity: detection.Critical,
				Summary:  "Container is crash looping",
			},
		},
	}

	steps := nextSteps(p, []detection.Finding{
		{
			Entity:   containerID,
			Reason:   reasons.CrashLoopBackOff,
			Severity: detection.Critical,
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
	containerID := inventory.CoreID(kube.KindContainer,
		"default", "app-pod/app")

	p := incident.Incident{
		Root: containerID,
		Members: map[detection.Key]detection.Finding{
			detection.Finding{
				Entity: containerID, Reason: reasons.OOMKilled,
			}.Key(): {
				Entity:   containerID,
				Reason:   reasons.OOMKilled,
				Severity: detection.Critical,
				Summary:  "Container was OOMKilled",
			},
		},
	}

	steps := nextSteps(p, []detection.Finding{
		{
			Entity:   containerID,
			Reason:   reasons.OOMKilled,
			Severity: detection.Critical,
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
	ssID := inventory.CoreID(kube.KindStatefulSet, "default",
		"db")

	p := incident.Incident{
		Root: ssID,
		Cause: &rootcause.CauseRecord{
			Change: &inventory.Change{Entity: ssID},
		},
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

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
	dsID := inventory.CoreID(kube.KindDaemonSet, "default",
		"monitoring")

	p := incident.Incident{
		Root: dsID,
		Cause: &rootcause.CauseRecord{
			Change: &inventory.Change{Entity: dsID},
		},
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

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
	podID := inventory.CoreID(kube.KindPod, "default",
		"test-pod")

	p := incident.Incident{
		Root:    podID,
		Members: map[detection.Key]detection.Finding{},
	}

	steps := nextSteps(p, []detection.Finding{})

	if len(steps) != 0 {
		t.Errorf("expected no steps without members, got %d", len(steps))
	}
}

// TestStepsForCausesWithoutFindings: a full claim and a webhook whose
// calls fail have no finding of their own to describe, so their steps
// read the object that needs the change.
func TestStepsForCausesWithoutFindings(t *testing.T) {
	claim := inventory.CoreID(kube.KindPVC, "data", "pgdata")
	hook := inventory.CoreID(kube.KindMutatingWebhook, "", "inject")
	cases := map[string]struct {
		root    inventory.EntityID
		cause   rootcause.CauseRecord
		command string
	}{
		"full claim": {claim, rootcause.CauseRecord{Root: claim,
			Rule: "claim-not-usable", Mode: detection.ModeVolumeFull,
			Summary: "persistentvolumeclaim pgdata (VolumeFull) explains 1"},
			"kubectl describe pvc pgdata -n data"},
		"webhook timing out": {hook, rootcause.CauseRecord{Root: hook,
			Rule: "webhook-rejects"},
			"kubectl get mutatingwebhookconfiguration inject -o yaml"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := incident.Incident{Root: c.root, Cause: &c.cause,
				Members: map[detection.Key]detection.Finding{}}
			steps := nextSteps(p, nil)
			if len(steps) != 1 || steps[0].Command != c.command ||
				steps[0].Mutating {
				t.Fatalf("steps = %+v, want %q", steps, c.command)
			}
		})
	}
}
