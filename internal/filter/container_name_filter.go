package filter

import (
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

type ContainerNameFilter struct{}

func (f ContainerNameFilter) Detect(ctx *Context) Status {
	container := ctx.Container.Container
	if len(ctx.Config.Suppression.ContainerNames) > 0 &&
		slices.Contains(ctx.Config.Suppression.ContainerNames, container.Name) {
		klog.InfoS(
			"skipping container as it is in the container ignore list",
			"container", container.Name)
		return StatusSkip
	}

	return StatusAlert
}

func (f ContainerNameFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
