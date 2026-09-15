package network

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Descriptor returns the stable metadata for the network monitor family.
func Descriptor() monitor.Descriptor {
	return monitor.Descriptor{
		Name:        "network-monitor",
		Description: "Detect service and network failures",
		Feature:     feature.NetworkDetection,
		Resources: []string{
			"service", "ingress", "networkpolicy",
		},
		Documentation: "/docs/general-configuration",
	}
}
