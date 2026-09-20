// Package policy contains deterministic pod and container detection rules.
//
// It has no Kubernetes client, lister, incident, or delivery dependency.
// Enrichment and reporting belong to the pod runtime boundary.
package policy

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

// DefaultNotReadyThreshold is the minimum time a running Pod must remain not
// ready before the default policy reports it.
const DefaultNotReadyThreshold = 60 * time.Second

// ContainerIssueReason returns the canonical reason represented by a
// container status. Startup baseline seeding and live evaluation both use
// this rule so they produce the same incident key.
func ContainerIssueReason(cs *corev1.ContainerStatus) string {
	if cs == nil {
		return ""
	}
	if waiting := cs.State.Waiting; waiting != nil {
		if waiting.Reason == constant.ReasonContainerCreating ||
			waiting.Reason == constant.ReasonPodInitializing {
			return ""
		}
		if waiting.Reason == constant.ReasonCrashLoopBackOff &&
			cs.LastTerminationState.Terminated != nil {
			return cs.LastTerminationState.Terminated.Reason
		}
		return waiting.Reason
	}
	if terminated := cs.State.Terminated; terminated != nil {
		if terminated.ExitCode == 0 ||
			terminated.Reason == constant.ReasonCompleted {
			return ""
		}
		return terminated.Reason
	}
	if cs.State.Running != nil && cs.RestartCount > 0 &&
		cs.LastTerminationState.Terminated != nil {
		return cs.LastTerminationState.Terminated.Reason
	}
	return ""
}

// ContainerIssueMessage returns the message paired with ContainerIssueReason.
func ContainerIssueMessage(cs *corev1.ContainerStatus) string {
	if cs == nil {
		return ""
	}
	if waiting := cs.State.Waiting; waiting != nil {
		if waiting.Reason == constant.ReasonContainerCreating ||
			waiting.Reason == constant.ReasonPodInitializing {
			return ""
		}
		if waiting.Reason == constant.ReasonCrashLoopBackOff &&
			cs.LastTerminationState.Terminated != nil {
			return cs.LastTerminationState.Terminated.Message
		}
		return waiting.Message
	}
	if terminated := cs.State.Terminated; terminated != nil {
		if terminated.ExitCode == 0 ||
			terminated.Reason == constant.ReasonCompleted {
			return ""
		}
		return terminated.Message
	}
	if cs.State.Running != nil && cs.RestartCount > 0 &&
		cs.LastTerminationState.Terminated != nil {
		return cs.LastTerminationState.Terminated.Message
	}
	return ""
}
