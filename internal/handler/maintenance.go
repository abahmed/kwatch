package handler

import (
	"strings"
	"time"
)

func (h *handler) inMaintenance(annotations map[string]string) bool {
	if h == nil || h.config == nil || !h.config.Maintenance.Enabled {
		return false
	}
	active := strings.EqualFold(strings.TrimSpace(
		annotations[h.config.Maintenance.Annotation],
	), "true")
	if active || h.config.Maintenance.UntilAnnotation == "" {
		return active
	}
	until, err := time.Parse(time.RFC3339,
		annotations[h.config.Maintenance.UntilAnnotation])
	return err == nil && h.now().Before(until)
}
