package filter

type EventFilter struct{}

func (f EventFilter) Execute(ctx *Context) bool {
	if ctx.EvType == "DELETED" {
		ctx.Memory.DelPod(ctx.Pod.Namespace, ctx.Pod.Name)
		return true
		//ctx.PodHasIssues = true
	}
	/*
		for _, c := range ctx.Pod.Status.ContainerStatuses {
			if ctx.EvType == "MODIFIED" &&
				ctx.Memory.HasPodContainer(
					ctx.Pod.Namespace,
					ctx.Pod.Name,
					c.Name) {
				continue
			}

			ctx.Memory.AddPodContainer(
				ctx.Pod.Namespace,
				ctx.Pod.Name,
				c.Name,
				&storage.ContainerState{
					RestartCount: c.RestartCount,
				})
			// ctx.StoreUpdated[c.Name] = true
		}*/

	return false
}
