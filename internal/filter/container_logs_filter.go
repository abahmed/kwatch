package filter

import (
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/k8s"
)

// unavailableLogsPrefix opens the body the kubelet returns -- with a 200 --
// when the runtime has already removed the container whose logs were asked
// for. Posted as-is it read as the application's own last words.
const unavailableLogsPrefix = "unable to retrieve container logs for"

func logsUnavailable(logs string) bool {
	return strings.HasPrefix(strings.TrimSpace(logs), unavailableLogsPrefix)
}

type ContainerLogsFilter struct{}

func (f ContainerLogsFilter) Detect(ctx *Context) Status {
	return StatusAlert
}

func (f ContainerLogsFilter) Enrich(ctx *Context) bool {
	if ctx.Container == nil || ctx.Pod == nil {
		return false
	}
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

	logs := ctx.LogCache.Do(
		LogCacheKey(ctx.Pod, container),
		func() string { return fetchContainerLogs(ctx) },
	)

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

// fetchContainerLogs reads a container's recent output from the kubelet.
func fetchContainerLogs(ctx *Context) string {
	container := ctx.Container.Container
	// Always fetch previous container logs when restarts exist so that
	// the crash output (not the current container's possibly-empty startup)
	// is included in the notification.
	previousLogs := container.RestartCount > 0

	logs := k8s.GetPodContainerLogs(
		ctx.Ctx,
		ctx.Client,
		ctx.Pod.Name,
		container.Name,
		ctx.Pod.Namespace,
		previousLogs,
		ctx.Config.MaxRecentLogLines)

	// After a crash the runtime may already have collected the previous
	// container; the current one's startup output is still worth having.
	if logsUnavailable(logs) {
		logs = k8s.GetPodContainerLogs(
			ctx.Ctx,
			ctx.Client,
			ctx.Pod.Name,
			container.Name,
			ctx.Pod.Namespace,
			!previousLogs,
			ctx.Config.MaxRecentLogLines)
	}
	if logsUnavailable(logs) {
		logs = ""
	}
	if logs == "" {
		return "[logs unavailable — kubelet timeout or container not yet " +
			"logged]"
	}
	return logs
}

func (f ContainerLogsFilter) Execute(ctx *Context) bool {
	return f.Enrich(ctx)
}
