package filter

type ContainerStateFilter struct{}

func (f ContainerStateFilter) Detect(ctx *Context) Status {
	if ctx.Container == nil {
		return StatusAlert
	}
	container := ctx.Container.Container

	switch {
	case container.State.Running != nil:
		ctx.Container.Status = "running"
	case container.State.Waiting != nil:
		ctx.Container.Status = "waiting"
	case container.State.Terminated != nil:
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

	// A container whose current state is Terminated with a clean exit
	// (Completed, or a graceful SIGTERM/exit-0) is not an alertable
	// failure regardless of restart history: kubelet only keeps the
	// current state as Terminated when it will not restart the container,
	// so a clean exit there always means the work finished successfully
	// (e.g. a Job, or an init container that failed once and then
	// succeeded on retry). Restart-count-based alerting is driven by the
	// Waiting/CrashLoopBackOff and non-zero-exit paths instead.
	if container.State.Terminated != nil &&
		(container.State.Terminated.Reason == "Completed" ||
			container.State.Terminated.ExitCode == 143 ||
			container.State.Terminated.ExitCode == 0) {
		return StatusSkip
	}

	return StatusAlert
}

func (f ContainerStateFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
