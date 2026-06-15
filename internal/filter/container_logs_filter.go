package filter

import (
	"context"

	"github.com/abahmed/kwatch/internal/k8s"
	"k8s.io/klog/v2"
)

type ContainerLogsFilter struct{}

func (f ContainerLogsFilter) Detect(ctx *Context) Status {
	return StatusAlert
}

func (f ContainerLogsFilter) Enrich(ctx *Context) bool {
	container := ctx.Container.Container

	if container.RestartCount == 0 && container.State.Waiting != nil {
		return false
	}

	// If the container terminated with ContainerStatusUnknown, logs are
	// unavailable — skip the API call entirely.
	if container.State.Terminated != nil &&
		container.State.Terminated.Reason == "ContainerStatusUnknown" {
		ctx.Container.Logs = ""
		return false
	}

	previousLogs := container.RestartCount > 0 && container.State.Running == nil

	logs := k8s.GetPodContainerLogs(
		context.Background(),
		ctx.Client,
		ctx.Pod.Name,
		container.Name,
		ctx.Pod.Namespace,
		previousLogs,
		ctx.Config.MaxRecentLogLines)

	for _, pattern := range ctx.Config.Suppression.LogPatterns {
		if pattern.MatchString(logs) {
			klog.InfoS(
				"skipping container logs as it matches the ignore log pattern",
				"container", container.Name)
			return true
		}
	}

	ctx.Container.Logs = logs
	return false
}

func (f ContainerLogsFilter) Execute(ctx *Context) bool {
	return f.Enrich(ctx)
}
