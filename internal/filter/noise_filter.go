package filter

import (
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

type NoiseFilter struct{}

var noiseReasons = []string{
	"Normal",
	"Scheduled",
	"Pulled",
	"Pulling",
}

func (f NoiseFilter) Detect(ctx *Context) Status {
	reason := ctx.Container.Reason
	if len(reason) == 0 {
		return StatusAlert
	}
	if slices.Contains(noiseReasons, reason) {
		klog.V(4).InfoS("skipping noise reason", "reason", reason)
		return StatusSkip
	}
	return StatusAlert
}

func (f NoiseFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
