package kube

import (
	"slices"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Container restart rules (spec.containers[].restartPolicyRules, feature
// ContainerRestartRules) restart a container whose exit code a rule
// names, whatever its restart policy says. The rules are checked in
// order and the first one whose exit codes match decides. A rule that
// lists the codes it restarts (operator In) declares those exits as the
// container's way of asking for a restart; NotIn is a catch-all like
// OnFailure and declares nothing.
const (
	// AttrRestartRule is true when the container's latest exit was
	// restarted by an In rule. Absent otherwise.
	AttrRestartRule = "restart.rule"
	// AttrRestartRuleRun is how many seconds that run lasted, when its
	// start and end are known.
	AttrRestartRuleRun = "restart.rule.run.seconds"
)

// setRestartRule records whether the container's latest exit is one its
// own restart rules restart.
func setRestartRule(
	attrs map[string]inventory.Value, c corev1.Container,
	s corev1.ContainerStatus,
) {
	if len(c.RestartPolicyRules) == 0 {
		return
	}
	exit := s.State.Terminated
	if exit == nil {
		exit = s.LastTerminationState.Terminated
	}
	if exit == nil || !restartedByRule(c.RestartPolicyRules, exit.ExitCode) {
		return
	}
	attrs[AttrRestartRule] = inventory.Bool(true)
	if !exit.StartedAt.IsZero() && !exit.FinishedAt.Before(&exit.StartedAt) {
		run := exit.FinishedAt.Sub(exit.StartedAt.Time)
		attrs[AttrRestartRuleRun] = inventory.Number(run.Seconds())
	}
}

// restartedByRule reports whether the first rule matching code is an
// In rule that restarts the container (or all of the pod's containers).
func restartedByRule(rules []corev1.ContainerRestartRule, code int32) bool {
	for _, rule := range rules {
		on := rule.ExitCodes
		if on == nil {
			continue
		}
		listed := slices.Contains(on.Values, code)
		switch on.Operator {
		case corev1.ContainerRestartRuleOnExitCodesOpIn:
			if listed {
				return restartAction(rule.Action)
			}
		case corev1.ContainerRestartRuleOnExitCodesOpNotIn:
			if !listed {
				return false
			}
		}
	}
	return false
}

func restartAction(action corev1.ContainerRestartRuleAction) bool {
	return action == corev1.ContainerRestartRuleActionRestart ||
		action == corev1.ContainerRestartRuleActionRestartAllContainers
}
