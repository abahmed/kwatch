package app

import (
	"context"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/startup"
)

type kubernetesRestartEvidence struct {
	client    kubernetes.Interface
	namespace string
}

func newKubernetesRestartEvidence(
	client kubernetes.Interface,
	namespace string,
) startup.EvidenceSource {
	return &kubernetesRestartEvidence{
		client: client, namespace: namespace,
	}
}

func (s *kubernetesRestartEvidence) ReadRestartEvidence(
	ctx context.Context,
	previous model.RuntimeSession,
) (startup.RestartEvidence, error) {
	evidence := startup.RestartEvidence{}
	if previous.PodName != "" {
		s.inspectPod(ctx, previous.PodName, &evidence)
		s.inspectPodEvents(ctx, previous.PodName, &evidence)
	}
	if previous.NodeName != "" {
		s.inspectNode(ctx, previous.NodeName, &evidence)
	}
	s.inspectLease(ctx, &evidence)
	return evidence, nil
}

func (s *kubernetesRestartEvidence) inspectPod(
	ctx context.Context,
	name string,
	evidence *startup.RestartEvidence,
) {
	pod, err := s.client.CoreV1().Pods(s.namespace).Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		if !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
			evidence.APIUnavailable = true
		}
		return
	}
	evidence.PodReason = pod.Status.Reason
	for _, status := range pod.Status.InitContainerStatuses {
		setTerminationReason(status, evidence)
	}
	for _, status := range pod.Status.ContainerStatuses {
		setTerminationReason(status, evidence)
	}
}

func setTerminationReason(
	status corev1.ContainerStatus,
	evidence *startup.RestartEvidence,
) {
	if status.State.Terminated != nil {
		evidence.ContainerReason = status.State.Terminated.Reason
	}
	if status.LastTerminationState.Terminated != nil &&
		evidence.ContainerReason == "" {
		evidence.ContainerReason =
			status.LastTerminationState.Terminated.Reason
	}
}

func (s *kubernetesRestartEvidence) inspectPodEvents(
	ctx context.Context,
	name string,
	evidence *startup.RestartEvidence,
) {
	selector := fields.OneTermEqualSelector(
		"involvedObject.name", name,
	).String()
	list, err := s.client.CoreV1().Events(s.namespace).List(
		ctx, metav1.ListOptions{FieldSelector: selector},
	)
	if err != nil {
		if !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
			evidence.APIUnavailable = true
		}
		return
	}
	for _, event := range list.Items {
		if event.Reason == "Evicted" || event.Reason == "OOMKilled" {
			evidence.PodReason = event.Reason
		}
	}
}

func (s *kubernetesRestartEvidence) inspectNode(
	ctx context.Context,
	name string,
	evidence *startup.RestartEvidence,
) {
	node, err := s.client.CoreV1().Nodes().Get(
		ctx, name, metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			evidence.NodeObserved = true
			evidence.NodeReason = "NotFound"
		} else if !apierrors.IsForbidden(err) {
			evidence.APIUnavailable = true
		}
		return
	}
	evidence.NodeObserved = true
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			evidence.NodeReady = condition.Status == corev1.ConditionTrue
			if !evidence.NodeReady {
				evidence.NodeReason = condition.Reason
			}
		}
		if condition.Status == corev1.ConditionTrue &&
			condition.Type != corev1.NodeReady {
			evidence.NodeReason = string(condition.Type)
		}
	}
}

func (s *kubernetesRestartEvidence) inspectLease(
	ctx context.Context,
	evidence *startup.RestartEvidence,
) {
	lease, err := s.client.CoordinationV1().Leases(s.namespace).Get(
		ctx, electionLeaseName(), metav1.GetOptions{},
	)
	if err != nil || lease.Spec.HolderIdentity == nil {
		if err != nil && !apierrors.IsNotFound(err) &&
			!apierrors.IsForbidden(err) {
			evidence.APIUnavailable = true
		}
		return
	}
	currentPod := os.Getenv("POD_NAME")
	holder := *lease.Spec.HolderIdentity
	if currentPod != "" && holder != "" && !strings.Contains(holder, currentPod) {
		evidence.LeaseLost = true
	}
}

var _ startup.EvidenceSource = (*kubernetesRestartEvidence)(nil)
