package filter

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

type ContainerNameFilter struct{}

func (f ContainerNameFilter) Execute(ctx *Context) bool {
	container := ctx.Container.Container
	if len(ctx.Config.IgnoreContainerNames) > 0 &&
		slices.Contains(ctx.Config.IgnoreContainerNames, container.Name) {
		logrus.Infof(
			"skipping container %s as it is in the container ignore list",
			container.Name)
		return true
	}

	return false
}
