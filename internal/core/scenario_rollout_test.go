package core

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notice"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/problem"
)

// A rollout changes the image; the new revision crash-loops while the old
// revision stays healthy. The rollout is the cause, named in the headline.
func TestEngineBadRolloutNamesTheChange(t *testing.T) {
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	old, oldRS := deployment("payments")
	old.Spec.Template = podTemplate("app:2.2")
	h.add(kube.DeploymentSchema(), old)
	h.add(kube.ReplicaSetSchema(), oldRS)
	h.add(kube.PodSchema{}, pod("payments-old", oldRS.Name, "n1", true,
		start))
	h.run(start.Add(time.Minute), 10*time.Second)

	updated := old.DeepCopy()
	updated.Spec.Template = podTemplate("app:2.3")
	h.engine.Submit(ctxBackground(), kube.NewTranslator(
		kube.DeploymentSchema()).Updated(old, updated, h.now)...)
	newRS := replicaSet("payments-9c", "payments")
	h.add(kube.ReplicaSetSchema(), newRS)
	h.add(kube.PodSchema{}, crashingPod("payments-new", newRS.Name, "n1",
		h.now.Add(40*time.Second)))

	h.run(start.Add(12*time.Minute), 10*time.Second)

	if len(h.decisions) == 0 {
		t.Fatal("expected the rollout problem to be announced")
	}
	first := h.decisions[0]
	if first.Action != problem.Announce || first.Problem.Cause == nil ||
		first.Problem.Cause.Change == nil {
		t.Fatalf("first decision should announce a change cause, got %+v",
			first.Problem.Cause)
	}
	title := h.messages[0].Title
	if !strings.Contains(title, "2.3") ||
		!strings.Contains(title, "payments") {
		t.Fatalf("title %q should name the workload and the new image",
			title)
	}
	for _, d := range h.decisions {
		if d.Problem.Root.Kind != kube.KindDeployment {
			t.Fatalf("decision rooted at %s", d.Problem.Root)
		}
	}
	t.Logf("%d messages; first:\n%s", len(h.messages),
		notice.Text(h.messages[0]))
	t.Logf("all:\n%s", joinTitles(h))
}

func podTemplate(image string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{Name: "app", Image: image}},
	}}
}

func replicaSet(name, deploymentName string) *appsv1.ReplicaSet {
	yes := true
	replicas := int32(1)
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "shop", UID: types.UID(name),
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Deployment", Name: deploymentName, Controller: &yes,
			}},
		},
		Spec: appsv1.ReplicaSetSpec{Replicas: &replicas},
	}
}

func crashingPod(
	name, owner, nodeName string, since time.Time,
) *corev1.Pod {
	p := pod(name, owner, nodeName, false, since)
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app", Image: "app:2.3", RestartCount: 4,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason: "CrashLoopBackOff",
		}},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason: "Error", ExitCode: 1,
				Message: "panic: missing key DB_PASSWORD_V2",
			},
		},
	}}
	return p
}
