// Package kubelet contains bounded Kubernetes kubelet access helpers.
package kubelet

import (
	"context"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// GetPodContainerLogs returns a bounded log tail for a Pod container.
func GetPodContainerLogs(
	ctx context.Context,
	c kubernetes.Interface,
	name string,
	container string,
	namespace string,
	previous bool,
	maxRecentLogLines int64,
) string {
	options := v1.PodLogOptions{
		Container: container,
		Previous:  previous,
	}

	if maxRecentLogLines != 0 {
		options.TailLines = &maxRecentLogLines
	} else {
		defaultTail := int64(500)
		options.TailLines = &defaultTail
	}
	limitBytes := int64(1024 * 1024)
	options.LimitBytes = &limitBytes

	logs, err := getContainerLogs(ctx, c, name, namespace, &options)
	if err != nil {
		klog.V(2).InfoS(
			"failed to get logs for container",
			"name", name,
			"container", container,
			"namespace", namespace,
			"error", err.Error())
		return ""
	}

	return string(logs)
}

// logFetchTimeout bounds one kubelet log read. Logs are optional context, so
// a slow kubelet must not hold a monitor worker for long.
const logFetchTimeout = 8 * time.Second

func getContainerLogs(
	ctx context.Context,
	c kubernetes.Interface,
	name string,
	namespace string,
	options *v1.PodLogOptions,
) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, logFetchTimeout)
	defer cancel()
	return c.CoreV1().Pods(namespace).GetLogs(name, options).DoRaw(cctx)
}
