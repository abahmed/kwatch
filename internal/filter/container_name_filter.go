package filter

import (
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

type ContainerNameFilter struct{}

func (f ContainerNameFilter) Execute(ctx *Context) bool {
	container := ctx.Container.Container
	if len(ctx.Config.IgnoreContainerNames) > 0 &&
		slices.Contains(ctx.Config.IgnoreContainerNames, container.Name) {
		klog.InfoS(
			"skipping container as it is in the container ignore list",
			"container", container.Name)
		return true
	}

	return false
}
