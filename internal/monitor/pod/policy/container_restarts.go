package policy

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type ContainerRestartsRule struct{}

func (rule ContainerRestartsRule) Detect(ctx *Context) Decision {
	if ctx.Container == nil {
		return DecisionAlert
	}
	container := ctx.Container.Container
	lastState := ctx.Container.LastState

	ctx.Container.HasRestarts = false
	if lastState == nil {
		// Without an earlier observation, a termination that finished
		// recently is still evidence of a restart; otherwise a crash that
		// restarts before the pod is processed would be missed.
		if ctx.Now != nil {
			ctx.Container.HasRestarts = recentlyRestarted(
				container, ctx.now(),
			)
		}
		return DecisionAlert
	}

	if container.RestartCount > lastState.RestartCount {
		ctx.Container.HasRestarts = true
	}

	return DecisionAlert
}

// recentRestartWindow is how recent an unobserved termination must be to
// count as a new restart; older ones were seen or baselined before.
const recentRestartWindow = 10 * time.Minute

func recentlyRestarted(
	container *corev1.ContainerStatus, now time.Time,
) bool {
	terminated := container.LastTerminationState.Terminated
	if container.RestartCount == 0 || terminated == nil ||
		terminated.FinishedAt.IsZero() {
		return false
	}
	age := now.Sub(terminated.FinishedAt.Time)
	return age >= 0 && age <= recentRestartWindow
}
