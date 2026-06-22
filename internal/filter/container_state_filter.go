package filter

type ContainerStateFilter struct{}

func (f ContainerStateFilter) Detect(ctx *Context) Status {
	container := ctx.Container.Container

	if container.State.Running != nil {
		ctx.Container.Status = "running"
	} else if container.State.Waiting != nil {
		ctx.Container.Status = "waiting"
	} else if container.State.Terminated != nil {
		ctx.Container.Status = "terminated"
	}

	if !ctx.Container.HasRestarts && container.State.Running != nil {
		return StatusSkip
	}

	if container.State.Waiting != nil &&
		(container.State.Waiting.Reason == "ContainerCreating" ||
			container.State.Waiting.Reason == "PodInitializing") {
		return StatusSkip
	}

	if container.State.Terminated != nil &&
		(container.State.Terminated.Reason == "Completed" ||
			container.State.Terminated.ExitCode == 143 ||
			container.State.Terminated.ExitCode == 0) &&
		!ctx.Container.HasRestarts {
		return StatusSkip
	}

	return StatusAlert
}

func (f ContainerStateFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
