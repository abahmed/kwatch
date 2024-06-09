package filter

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

type ContainerReasonsFilter struct{}

func (f ContainerReasonsFilter) Execute(ctx *Context) bool {
	container := ctx.Container.Container

	if container.State.Waiting != nil {
		ctx.Container.Reason = container.State.Waiting.Reason
		ctx.Container.Msg = container.State.Waiting.Message
	} else if container.State.Terminated != nil {
		ctx.Container.Reason = container.State.Terminated.Reason
		ctx.Container.Msg = container.State.Terminated.Message
		ctx.Container.ExitCode = container.State.Terminated.ExitCode
		ctx.Container.LastTerminatedOn = container.State.Terminated.StartedAt.Time
	}

	if (ctx.Container.Reason == "CrashLoopBackOff" ||
		ctx.Container.HasRestarts) &&
		container.LastTerminationState.Terminated != nil {
		ctx.Container.Reason =
			container.LastTerminationState.Terminated.Reason
		ctx.Container.Msg =
			container.LastTerminationState.Terminated.Message
		ctx.Container.ExitCode =
			container.LastTerminationState.Terminated.ExitCode
		ctx.Container.LastTerminatedOn =
			container.LastTerminationState.Terminated.StartedAt.Time
	}

	if len(ctx.Config.AllowedReasons) > 0 &&
		!slices.Contains(ctx.Config.AllowedReasons, ctx.Container.Reason) {
		logrus.Infof(
			"skipping reason %s as it is not in the reason allow list",
			ctx.Container.Reason)
		return true
	}

	if len(ctx.Config.ForbiddenReasons) > 0 &&
		slices.Contains(ctx.Config.ForbiddenReasons, ctx.Container.Reason) {
		logrus.Infof(
			"skipping reason %s as it is in the reason forbid list",
			ctx.Container.Reason)
		return true
	}

	lastState := ctx.Memory.GetPodContainer(ctx.Pod.Namespace,
		ctx.Pod.Name,
		container.Name)

	if lastState != nil {
		if lastState.LastTerminatedOn == ctx.Container.LastTerminatedOn {
			return true
		}

		if lastState.Reason == ctx.Container.Reason &&
			lastState.Msg == ctx.Container.Msg &&
			lastState.ExitCode == ctx.Container.ExitCode {
			return true
		}
	}

	return false
}
