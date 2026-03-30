package filter

import (
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

type NamespaceFilter struct{}

func (f NamespaceFilter) Execute(ctx *Context) bool {
	// filter by namespaces in config if specified
	if len(ctx.Config.AllowedNamespaces) > 0 &&
		!slices.Contains(ctx.Config.AllowedNamespaces, ctx.Pod.Namespace) {
		klog.InfoS(
			"skipping namespace as it is not in the namespace allow list",
			"namespace", ctx.Pod.Namespace)
		return true
	}

	if len(ctx.Config.ForbiddenNamespaces) > 0 &&
		slices.Contains(ctx.Config.ForbiddenNamespaces, ctx.Pod.Namespace) {
		klog.InfoS(
			"skipping namespace as it is in the namespace forbid list",
			"namespace", ctx.Pod.Namespace)
		return true
	}

	return false
}
