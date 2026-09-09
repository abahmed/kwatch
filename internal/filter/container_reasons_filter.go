package filter

import (
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
)

type ContainerReasonsFilter struct{}

func (f ContainerReasonsFilter) Detect(ctx *Context) Status {
	if ctx.Container == nil {
		return StatusAlert
	}
	resolveContainerReason(ctx)
	if status, decided := reasonListStatus(ctx); decided {
		return status
	}
	return repeatStatus(ctx)
}

// resolveContainerReason picks the reason, message and exit code that best
// describe why a container is unhealthy: its current state, corrected to the
// base reason the startup baseline uses, then the last termination when the
// current state is only the back-off wrapper around it.
func resolveContainerReason(ctx *Context) {
	container := ctx.Container.Container

	if container.State.Waiting != nil {
		ctx.Container.Reason = container.State.Waiting.Reason
		ctx.Container.Msg = container.State.Waiting.Message
	} else if container.State.Terminated != nil {
		ctx.Container.Reason = container.State.Terminated.Reason
		ctx.Container.Msg = container.State.Terminated.Message
		ctx.Container.ExitCode = container.State.Terminated.ExitCode
		ctx.Container.LastTerminatedOn =
			container.State.Terminated.StartedAt.Time
	}
	// Keep the base reason aligned with ContainerIssueReason, which the
	// startup baseline also uses, so a seeded key matches this one.
	if base := ContainerIssueReason(container); base != "" &&
		container.State.Waiting != nil &&
		container.State.Waiting.Reason == constant.ReasonCrashLoopBackOff {
		ctx.Container.Reason = base
	}

	if (ctx.Container.Reason == constant.ReasonCrashLoopBackOff ||
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
}

// reasonListStatus applies the operator's reason allow and forbid lists.
// decided is false when neither list has an opinion.
func reasonListStatus(ctx *Context) (Status, bool) {
	if len(ctx.Config.AllowedReasons) > 0 &&
		!slices.Contains(ctx.Config.AllowedReasons, ctx.Container.Reason) {
		klog.InfoS(
			"skipping reason as it is not in the reason allow list",
			"reason", ctx.Container.Reason)
		return StatusSkip, true
	}

	if len(ctx.Config.ForbiddenReasons) > 0 &&
		slices.Contains(ctx.Config.ForbiddenReasons, ctx.Container.Reason) {
		klog.InfoS(
			"skipping reason as it is in the reason forbid list",
			"reason", ctx.Container.Reason)
		return StatusSkip, true
	}
	return StatusAlert, false
}

// repeatStatus decides whether this sighting says anything the last one did
// not. Seeing the same termination twice is never news; a new one always is.
//
// There used to be a second test here: a *different* termination with the
// same reason, message and exit code was dropped if it fell inside
// correlation.cooldownMinutes. That silently defeated the engine's own
// post-resolve cooldown, which exists to revive the incident quietly and keep
// counting. A container that crashed again four minutes after its incident
// resolved never reached the engine at all, so the incident was cleaned up as
// recovered and the next crash opened a brand-new one on a new thread -- the
// exact flip-flop both cooldowns were meant to prevent. Deduplication of
// repeated crashes belongs in one place, and that place is the engine.
func repeatStatus(ctx *Context) Status {
	lastState := ctx.Container.LastState
	if lastState == nil {
		return StatusAlert
	}
	if lastState.LastTerminatedOn.Equal(ctx.Container.LastTerminatedOn) {
		return StatusSkip
	}
	return StatusAlert
}

func (f ContainerReasonsFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
