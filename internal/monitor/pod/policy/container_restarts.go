package policy

type ContainerRestartsRule struct{}

func (rule ContainerRestartsRule) Detect(ctx *Context) Decision {
	if ctx.Container == nil {
		return DecisionAlert
	}
	container := ctx.Container.Container
	lastState := ctx.Container.LastState

	ctx.Container.HasRestarts = false
	if lastState == nil {
		return DecisionAlert
	}

	if container.RestartCount > lastState.RestartCount {
		ctx.Container.HasRestarts = true
	}

	return DecisionAlert
}
