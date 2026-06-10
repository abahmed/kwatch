package filter

type ContainerRestartsFilter struct{}

func (f ContainerRestartsFilter) Execute(ctx *Context) bool {
	container := ctx.Container.Container
	lastState := ctx.Container.LastState

	ctx.Container.HasRestarts = false
	if lastState == nil {
		return false
	}

	if container.RestartCount > lastState.RestartCount {
		ctx.Container.HasRestarts = true
	}

	return false
}
