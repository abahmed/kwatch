package policy

import "github.com/abahmed/kwatch/internal/filter"

type ContainerMessageRule struct{}

func (rule ContainerMessageRule) Detect(ctx *Context) Decision {
	if ctx.Container == nil || ctx.Container.Container == nil {
		return DecisionAlert
	}

	cs := ctx.Container.Container
	msg := ""
	if cs.State.Waiting != nil {
		msg = cs.State.Waiting.Message
	} else if cs.State.Terminated != nil {
		msg = cs.State.Terminated.Message
	}

	if msg == "" {
		return DecisionAlert
	}

	if filter.MatchesContainerMessage(ctx.suppressionIndex(), msg) {
		return DecisionSuppress
	}

	return DecisionAlert
}
