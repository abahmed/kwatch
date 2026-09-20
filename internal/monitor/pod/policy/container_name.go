package policy

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/filter"
)

type ContainerNameRule struct{}

func (rule ContainerNameRule) Detect(ctx *Context) Decision {
	if ctx.Container == nil {
		return DecisionAlert
	}
	container := ctx.Container.Container
	if filter.MatchesContainerName(
		ctx.suppressionIndex(),
		container.Name,
	) {
		klog.InfoS(
			"skipping container as it is in the container ignore list",
			"container", container.Name)
		return DecisionSuppress
	}

	return DecisionAlert
}
