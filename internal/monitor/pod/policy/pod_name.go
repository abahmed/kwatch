package policy

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/filter"
)

type PodNameRule struct{}

func (rule PodNameRule) Detect(ctx *Context) Decision {
	if ctx.Pod == nil {
		return DecisionAlert
	}
	if filter.MatchesPodName(ctx.suppressionIndex(), ctx.Pod.Name) {
		klog.InfoS(
			"skipping pod as it is in the ignore pod name list",
			"pod", ctx.Pod.Name)
		return DecisionSuppress
	}

	return DecisionAlert
}
