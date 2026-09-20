package enrichment

import (
	"context"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/kubelet"
)

// unavailableLogsPrefix opens the body the kubelet returns -- with a 200 --
// when the runtime has already removed the container whose logs were asked
// for. Posted as-is it read as the application's own last words.
const unavailableLogsPrefix = "unable to retrieve container logs for"

// LogsUnavailable identifies the kubelet response used when the container's
// previous log stream has already been removed.
func LogsUnavailable(logs string) bool {
	return strings.HasPrefix(strings.TrimSpace(logs), unavailableLogsPrefix)
}

type ContainerLogsEnricher struct{}

func (enricher ContainerLogsEnricher) Enrich(ctx *Context) bool {
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

	if filter.MatchesLog(ctx.Runtime.Scope().SuppressionIndex(), logs) {
		klog.InfoS(
			"skipping container logs as it matches the ignore log pattern",
			"container", container.Name)
		return true
	}

	ctx.Container.Logs = logs
	return false
}

// fetchContainerLogs reads a container's recent output from the kubelet.
func fetchContainerLogs(ctx *Context) string {
	container := ctx.Container.Container
	fetcher := ctx.ContainerLogs
	if fetcher == nil {
		if ctx.Client == nil {
			return "[logs unavailable — kubelet timeout or container not yet " +
				"logged]"
		}
		fetcher = func(
			fetchCtx context.Context,
			podName, containerName, namespace string,
			previous bool,
			maxLines int64,
		) string {
			return kubelet.GetPodContainerLogs(
				fetchCtx,
				ctx.Client,
				podName,
				containerName,
				namespace,
				previous,
				maxLines,
			)
		}
	}
	fetchCtx := ctx.Ctx
	if fetchCtx == nil {
		fetchCtx = context.Background()
	}
	// Always fetch previous container logs when restarts exist so that
	// the crash output (not the current container's possibly-empty startup)
	// is included in the notification.
	previousLogs := container.RestartCount > 0

	logs := fetcher(
		fetchCtx,
		ctx.Pod.Name,
		container.Name,
		ctx.Pod.Namespace,
		previousLogs,
		ctx.Runtime.Monitors().MaxRecentLogLines())

	// After a crash the runtime may already have collected the previous
	// container; the current one's startup output is still worth having.
	if LogsUnavailable(logs) {
		logs = fetcher(
			fetchCtx,
			ctx.Pod.Name,
			container.Name,
			ctx.Pod.Namespace,
			!previousLogs,
			ctx.Runtime.Monitors().MaxRecentLogLines())
	}
	if LogsUnavailable(logs) {
		logs = ""
	}
	if logs == "" {
		return "[logs unavailable — kubelet timeout or container not yet " +
			"logged]"
	}
	return logs
}
