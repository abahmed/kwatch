package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notification"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An application crashes on a healthy node with no recent change. No
// cause is invented: the workload is the root and the message says the
// application itself is the most likely source.
func TestEngineAppCrashHasNoInventedCause(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("reports")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{},
		crashingPod("reports-a", rs.Name, "n1", start),
		crashingPod("reports-b", rs.Name, "n1", start))

	h.run(start.Add(10*time.Minute), 10*time.Second)

	if len(h.decisions) != 1 {
		t.Fatalf("got %d messages, want 1:\n%s", len(h.decisions),
			joinTitles(h))
	}
	p := h.decisions[0].Incident
	if p.Cause != nil {
		t.Fatalf("invented cause %q for an application crash",
			p.Cause.Summary)
	}
	if p.Root.Kind != kube.KindDeployment || p.Root.Name != "reports" {
		t.Fatalf("root = %s, want deployment reports", p.Root)
	}
	text := notification.Text(h.messages[0])
	if !strings.Contains(text, "couldn't find an outside cause") ||
		!strings.Contains(text, "kubectl logs") {
		t.Fatalf("message should say no outside cause and show logs:\n%s",
			text)
	}
}

// A pod references a Secret that does not exist. The Secret is the root.
func TestEngineMissingSecretIsTheRoot(t *testing.T) {
	start := time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("billing")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	p := pod("billing-a", rs.Name, "n1", false, start)
	p.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "billing-creds",
			},
		},
	}}
	p.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{
			Reason:  "CreateContainerConfigError",
			Message: `secret "billing-creds" not found`,
		},
	}
	h.add(kube.PodSchema{}, p)

	h.run(start.Add(6*time.Minute), 10*time.Second)

	if len(h.decisions) != 1 {
		t.Fatalf("got %d messages, want 1:\n%s", len(h.decisions),
			joinTitles(h))
	}
	root := h.decisions[0].Incident.Root
	if root.Kind != kube.KindSecret || root.Name != "billing-creds" {
		t.Fatalf("root = %s, want secret billing-creds", root)
	}
	t.Logf("\n%s", notification.Text(h.messages[0]))
}
