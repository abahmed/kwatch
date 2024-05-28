package filter

type ContainerStateFilter struct{}

func (f ContainerStateFilter) Execute(ctx *Context) bool {
	container := ctx.Container.Container

	if container.State.Running != nil {
		ctx.Container.Status = "running"
	} else if container.State.Waiting != nil {
		ctx.Container.Status = "waiting"
	} else if container.State.Terminated != nil {
		ctx.Container.Status = "terminated"
	}

	if !ctx.Container.HasRestarts && container.State.Running != nil {
		return true
	}

	if container.State.Waiting != nil &&
		(container.State.Waiting.Reason == "ContainerCreating" ||
			container.State.Waiting.Reason == "PodInitializing") {
		return true
	}

	if container.State.Terminated != nil &&
		(container.State.Terminated.Reason == "Completed" ||
			// 143 is the exit code for graceful termination
			container.State.Terminated.ExitCode == 143 ||
			// 0 is the exit code for purpose stop
			container.State.Terminated.ExitCode == 0) {
		return true
	}

	return false
}
