package policy

import (
	"strings"
	"time"
)

// MaintenanceRule suppresses pod/container symptoms while the operator has
// explicitly marked the pod for maintenance. Ordinary restarts remain visible.
type MaintenanceRule struct{}

func (rule MaintenanceRule) Detect(ctx *Context) Decision {
	maintenance := ctx.maintenance()
	if ctx.Pod == nil || !maintenance.Enabled {
		return DecisionAlert
	}
	annotations := ctx.Pod.Annotations
	active := strings.EqualFold(strings.TrimSpace(
		annotations[maintenance.Annotation],
	), "true")
	if !active && maintenance.UntilAnnotation != "" {
		until, err := time.Parse(
			time.RFC3339, annotations[maintenance.UntilAnnotation],
		)
		active = err == nil && ctx.now().Before(until)
	}
	if active {
		return DecisionSuppress
	}
	return DecisionAlert
}
