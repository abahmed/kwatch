package network

import (
	corev1 "k8s.io/api/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/model"
)

// enrichServiceEndpointFinding adds cached backend evidence to a finding.
// A shared node is named only when every unready pod is on it and the
// node's own Ready condition reports a failure.
func enrichServiceEndpointFinding(
	finding *model.Observation,
	pods []*corev1.Pod,
	nodes corev1lister.NodeLister,
) {
	if finding == nil {
		return
	}
	facts := finding.Facts
	facts.BackendsObserved = true
	sharedNode := ""
	allOnOneNode := true
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		facts.BackendPods++
		if podReady(pod) {
			continue
		}
		facts.UnreadyBackendPods++
		switch {
		case pod.Spec.NodeName == "":
			allOnOneNode = false
		case sharedNode == "":
			sharedNode = pod.Spec.NodeName
		case sharedNode != pod.Spec.NodeName:
			allOnOneNode = false
		}
	}
	if facts.UnreadyBackendPods == 0 ||
		!allOnOneNode || nodes == nil {
		finding.Facts = facts
		return
	}
	node, err := nodes.Get(sharedNode)
	if err != nil {
		finding.Facts = facts
		return
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady &&
			condition.Status != corev1.ConditionTrue {
			facts.SharedFailingNode = sharedNode
			facts.NodeFailureReason = condition.Reason
			break
		}
	}
	finding.Facts = facts
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// hasLiveBackend reports whether any selected pod is not being deleted.
func hasLiveBackend(pods []*corev1.Pod) bool {
	for _, pod := range pods {
		if pod.DeletionTimestamp == nil {
			return true
		}
	}
	return false
}
