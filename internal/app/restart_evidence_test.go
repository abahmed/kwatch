package app

import (
	"context"
	"testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/model"
)

func TestKubernetesRestartEvidenceClassifiesTermination(t *testing.T) {
	t.Setenv("POD_NAME", "kwatch-new")
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kwatch-old", Namespace: "kwatch",
			},
			Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
				{LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason: "OOMKilled",
					},
				}},
			}},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-a"},
			Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse,
					Reason: "KubeletNotReady"},
			}},
		},
		&coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name: electionLeaseName(), Namespace: "kwatch",
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: pointer("kwatch-new"),
			},
		},
	)
	source := newKubernetesRestartEvidence(client, "kwatch")
	evidence, err := source.ReadRestartEvidence(
		context.Background(), model.RuntimeSession{
			PodName: "kwatch-old", NodeName: "worker-a",
		},
	)
	if err != nil {
		t.Fatalf("ReadRestartEvidence() error = %v", err)
	}
	if evidence.ContainerReason != "OOMKilled" {
		t.Fatalf("container reason = %q", evidence.ContainerReason)
	}
	if !evidence.NodeObserved || evidence.NodeReady {
		t.Fatalf("unexpected node evidence: %+v", evidence)
	}
}

func TestKubernetesRestartEvidenceMarksMissingNode(t *testing.T) {
	source := newKubernetesRestartEvidence(
		fake.NewSimpleClientset(), "kwatch",
	)
	evidence, err := source.ReadRestartEvidence(
		context.Background(), model.RuntimeSession{NodeName: "worker-gone"},
	)
	if err != nil {
		t.Fatalf("ReadRestartEvidence() error = %v", err)
	}
	if !evidence.NodeObserved || evidence.NodeReason != "NotFound" {
		t.Fatalf("unexpected missing node evidence: %+v", evidence)
	}
}

func pointer(value string) *string { return &value }
