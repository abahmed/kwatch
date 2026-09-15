package node

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Descriptor returns the stable metadata for the node monitor family.
func Descriptor() monitor.Descriptor {
	return monitor.Descriptor{
		Name:          "node-monitor",
		Description:   "Detect node readiness and resource failures",
		Feature:       feature.NodeDetection,
		Resources:     []string{"node"},
		Documentation: "/docs/general-configuration",
	}
}
