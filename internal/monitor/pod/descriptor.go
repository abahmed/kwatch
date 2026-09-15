package pod

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Descriptor returns the stable metadata for the pod monitor family.
func Descriptor() monitor.Descriptor {
	return monitor.Descriptor{
		Name:          "pod-monitor",
		Description:   "Detect pod and container failures",
		Feature:       feature.PodDetection,
		Resources:     []string{"pod"},
		Documentation: "/docs/general-configuration",
	}
}
