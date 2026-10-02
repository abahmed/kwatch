package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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
		context.Background(), runtimeSession{
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
		context.Background(), runtimeSession{NodeName: "worker-gone"},
	)
	if err != nil {
		t.Fatalf("ReadRestartEvidence() error = %v", err)
	}
	if !evidence.NodeObserved || evidence.NodeReason != "NotFound" {
		t.Fatalf("unexpected missing node evidence: %+v", evidence)
	}
}

func pointer(value string) *string { return &value }

func failingEvidence(verbs ...string) *fake.Clientset {
	client := fake.NewSimpleClientset()
	for _, verb := range verbs {
		client.PrependReactor(verb, "*", func(
			k8stesting.Action,
		) (bool, runtime.Object, error) {
			return true, nil, errors.New("api down")
		})
	}
	return client
}

func TestKubernetesRestartEvidenceFlagsUnavailableAPI(t *testing.T) {
	source := newKubernetesRestartEvidence(
		failingEvidence("get", "list"), "kwatch")

	evidence, err := source.ReadRestartEvidence(context.Background(),
		runtimeSession{PodName: "old", NodeName: "n1"})

	require.NoError(t, err)
	require.True(t, evidence.APIUnavailable)
}

func TestKubernetesRestartEvidenceIgnoresForbiddenReads(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("*", "*", func(
		k8stesting.Action,
	) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "pods"}, "old", errors.New("no"))
	})

	evidence, err := newKubernetesRestartEvidence(client, "kwatch").
		ReadRestartEvidence(context.Background(),
			runtimeSession{PodName: "old", NodeName: "n1"})

	require.NoError(t, err)
	require.False(t, evidence.APIUnavailable)
}

func TestKubernetesRestartEvidenceReadsEvictionEvents(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: "kwatch"},
		Reason:     "Evicted",
		InvolvedObject: corev1.ObjectReference{
			Name: "old", Kind: "Pod",
		},
	})

	evidence, _ := newKubernetesRestartEvidence(client, "kwatch").
		ReadRestartEvidence(context.Background(),
			runtimeSession{PodName: "old"})

	require.Equal(t, "Evicted", evidence.PodReason)
}

func TestKubernetesRestartEvidenceRecordsPodStatusReason(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "kwatch"},
		Status: corev1.PodStatus{
			Reason: "Evicted",
			InitContainerStatuses: []corev1.ContainerStatus{{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Reason: "Error",
					},
				},
			}},
		},
	})

	evidence, _ := newKubernetesRestartEvidence(client, "kwatch").
		ReadRestartEvidence(context.Background(),
			runtimeSession{PodName: "old"})

	require.Equal(t, "Evicted", evidence.PodReason)
	require.Equal(t, "Error", evidence.ContainerReason)
}
