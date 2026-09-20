package policy

import (
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

type NoiseRule struct{}

var NoiseReasons = []string{
	"Normal",
	"Scheduled",
	"Pulled",
	"Pulling",
}

func (rule NoiseRule) Detect(ctx *Context) Decision {
	if ctx.Container == nil {
		return DecisionAlert
	}
	reason := ctx.Container.Reason
	if len(reason) == 0 {
		return DecisionAlert
	}
	if slices.Contains(NoiseReasons, reason) {
		klog.V(4).InfoS("skipping noise reason", "reason", reason)
		return DecisionSuppress
	}
	return DecisionAlert
}
