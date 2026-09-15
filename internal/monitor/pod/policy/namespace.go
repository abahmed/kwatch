package policy

import (
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

type NamespaceRule struct{}

func (rule NamespaceRule) Detect(ctx *Context) Decision {
	if ctx.Pod == nil {
		return DecisionAlert
	}
	allowed := ctx.allowedNamespaces()
	if len(allowed) > 0 && !slices.Contains(allowed, ctx.Pod.Namespace) {
		klog.InfoS(
			"skipping namespace as it is not in the namespace allow list",
			"namespace", ctx.Pod.Namespace)
		return DecisionSuppress
	}

	forbidden := ctx.forbiddenNamespaces()
	if len(forbidden) > 0 && slices.Contains(forbidden, ctx.Pod.Namespace) {
		klog.InfoS(
			"skipping namespace as it is in the namespace forbid list",
			"namespace", ctx.Pod.Namespace)
		return DecisionSuppress
	}

	return DecisionAlert
}
