package filter

import (
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
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

	for _, pattern := range ctx.Config.IgnoreLogPatternsCompiled {
		if pattern.MatchString(logs) {
			logrus.Infof(
				"skipping container %s logs as it matches the ignore log pattern",
				container.Name)
			return true
		}
	}

	ctx.Container.Logs = logs
	return false
}
