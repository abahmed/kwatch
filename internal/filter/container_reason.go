package filter

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

// ContainerIssueReason is the reason a container's current status reports, or
// "" when the status is not a failure.
//
// It existed twice: once in the container detector and once in the startup
// baseline seeder, with the transient-state and clean-exit rules written out
// separately in each. Two copies of "which reason is this?" is how a seeded
// key and a live key come to disagree, and a key that disagrees means the
// baseline does not suppress the incident it was recorded for -- the exact
// shape of "kwatch re-announced everything after a restart".
//
// The live detector applies one further rule this cannot: once it has seen
// the restart count rise it folds the reason to the previous termination.
// That needs an earlier observation, so it belongs to the detector, not here.
func ContainerIssueReason(cs *corev1.ContainerStatus) string {
	if cs == nil {
		return ""
	}
	if w := cs.State.Waiting; w != nil {
		if w.Reason == constant.ReasonContainerCreating ||
			w.Reason == constant.ReasonPodInitializing {
			return ""
		}
		// A crash loop reports "CrashLoopBackOff" while it waits to restart.
		// The actionable reason is why it died, which is the previous
		// termination.
		if w.Reason == constant.ReasonCrashLoopBackOff &&
			cs.LastTerminationState.Terminated != nil {
			return cs.LastTerminationState.Terminated.Reason
		}
		return w.Reason
	}
	if t := cs.State.Terminated; t != nil {
		if t.ExitCode == 0 || t.Reason == constant.ReasonCompleted {
			return ""
		}
		return t.Reason
	}
	// Running again after a crash: the incident is about the crash.
	if cs.State.Running != nil && cs.RestartCount > 0 &&
		cs.LastTerminationState.Terminated != nil {
		return cs.LastTerminationState.Terminated.Reason
	}
	return ""
}

// ContainerIssueMessage is the message that goes with ContainerIssueReason,
// read from the same branch of the status.
//
// The startup baseline needs it because the incident key is not built from
// the reason alone: a global image-pull failure (a registry rate limit, a DNS
// or TLS error) is keyed cluster-wide, and that classification is made from
// this message. Seeding without it produced a namespace-scoped key for an
// incident the live path keys globally, so the baseline missed and kwatch
// announced a pre-existing image-pull failure on every restart.
func ContainerIssueMessage(cs *corev1.ContainerStatus) string {
	if cs == nil {
		return ""
	}
	if w := cs.State.Waiting; w != nil {
		if w.Reason == constant.ReasonContainerCreating ||
			w.Reason == constant.ReasonPodInitializing {
			return ""
		}
		if w.Reason == constant.ReasonCrashLoopBackOff &&
			cs.LastTerminationState.Terminated != nil {
			return cs.LastTerminationState.Terminated.Message
		}
		return w.Message
	}
	if t := cs.State.Terminated; t != nil {
		if t.ExitCode == 0 || t.Reason == constant.ReasonCompleted {
			return ""
		}
		return t.Message
	}
	if cs.State.Running != nil && cs.RestartCount > 0 &&
		cs.LastTerminationState.Terminated != nil {
		return cs.LastTerminationState.Terminated.Message
	}
	return ""
}
