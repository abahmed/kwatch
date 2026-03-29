package filter

import (
	"k8s.io/klog/v2"
)

type PodNameFilter struct{}

func (f PodNameFilter) Execute(ctx *Context) bool {
	for _, pattern := range ctx.Config.IgnorePodNamePatterns {
		if pattern.MatchString(ctx.Pod.Name) {
			klog.InfoS(
				"skipping pod as it is in the ignore pod name list",
				"pod", ctx.Pod.Name)
			return true
		}
	}

	return false
}
