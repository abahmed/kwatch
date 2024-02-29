package filter

type ContainerRestartsFilter struct{}

func (f ContainerRestartsFilter) Execute(ctx *Context) bool {
	container := ctx.Container.Container

	lastState := ctx.Memory.GetPodContainer(ctx.Pod.Namespace,
		ctx.Pod.Name,
		container.Name)

	ctx.Container.HasRestarts = false
	if lastState == nil {
		return false
	}

	if container.RestartCount > lastState.RestartCount {
		ctx.Container.HasRestarts = true
	}

	return false
}
