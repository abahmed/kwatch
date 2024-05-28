package filter

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

type NsFilter struct{}

func (f NsFilter) Execute(ctx *Context) bool {
	// filter by namespaces in config if specified
	if len(ctx.Config.AllowedNamespaces) > 0 &&
		!slices.Contains(ctx.Config.AllowedNamespaces, ctx.Pod.Namespace) {
		logrus.Infof(
			"skip namespace %s as not in namespace allow list",
			ctx.Pod.Namespace)
		return true
	}

	if len(ctx.Config.ForbiddenNamespaces) > 0 &&
		slices.Contains(ctx.Config.ForbiddenNamespaces, ctx.Pod.Namespace) {
		logrus.Infof(
			"skip namespace %s as in namespace forbid list",
			ctx.Pod.Namespace)
		return true
	}

	return false
}
