package filter

type ContainerRestartsFilter struct{}

func (f ContainerRestartsFilter) Detect(ctx *Context) Status {
	container := ctx.Container.Container
	lastState := ctx.Container.LastState

	ctx.Container.HasRestarts = false
	if lastState == nil {
		return StatusAlert
	}

	if container.RestartCount > lastState.RestartCount {
		ctx.Container.HasRestarts = true
	}

	return StatusAlert
}

func (f ContainerRestartsFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
