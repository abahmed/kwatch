package filter

import (
	"strings"
	"time"
)

// MaintenanceFilter suppresses pod/container symptoms while the operator has
// explicitly marked the pod for maintenance. Ordinary restarts remain visible.
type MaintenanceFilter struct{}

func (f MaintenanceFilter) Detect(ctx *Context) Status {
	if ctx.Pod == nil || ctx.Config == nil || !ctx.Config.Maintenance.Enabled {
		return StatusAlert
	}
	annotations := ctx.Pod.Annotations
	active := strings.EqualFold(strings.TrimSpace(
		annotations[ctx.Config.Maintenance.Annotation],
	), "true")
	if !active && ctx.Config.Maintenance.UntilAnnotation != "" {
		until, err := time.Parse(time.RFC3339, annotations[ctx.Config.Maintenance.UntilAnnotation])
		active = err == nil && ctx.now().Before(until)
	}
	if active {
		return StatusSkip
	}
	return StatusAlert
}

func (f MaintenanceFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
