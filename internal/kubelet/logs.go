// Package kubelet contains bounded Kubernetes kubelet access helpers.
package kubelet

import (
	"context"
	"fmt"
	"strings"
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

func getContainerLogs(
	ctx context.Context,
	c kubernetes.Interface,
	name string,
	namespace string,
	options *v1.PodLogOptions,
) ([]byte, error) {
	// Attempt with 15s timeout; retry once on timeout if context allows.
	for attempt := 0; attempt < 2; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		logs, err := c.CoreV1().Pods(
			namespace,
		).GetLogs(
			name,
			options,
		).DoRaw(
			cctx,
		)
		cancel()

		if err == nil {
			return logs, nil
		}
		if attempt == 0 && cctx.Err() == nil && isTimeoutError(err) {
			klog.V(2).InfoS("log fetch timeout, retrying",
				"container", name, "namespace", namespace)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf(
		"log fetch failed after retries for container %s",
		name,
	)
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "i/o timeout")
}
