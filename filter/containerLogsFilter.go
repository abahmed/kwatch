package filter

import (
	"github.com/abahmed/kwatch/util"
)

type ContainerLogsFilter struct{}

func (f ContainerLogsFilter) Execute(ctx *Context) bool {
	container := ctx.Container.Container

	if container.RestartCount == 0 && container.State.Waiting != nil {
		return false
	}

	previousLogs := false
	if ctx.Container.HasRestarts && container.State.Running != nil {
		previousLogs = true
	}

	logs := util.GetPodContainerLogs(
		ctx.Client,
		ctx.Pod.Name,
		container.Name,
		ctx.Pod.Namespace,
		previousLogs,
		ctx.Config.MaxRecentLogLines)

	ctx.Container.Logs = logs
	return false
}
