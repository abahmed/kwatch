package cluster

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Descriptor returns the stable metadata for cluster-resource monitoring.
func Descriptor() monitor.Descriptor {
	return monitor.Descriptor{
		Name:        "cluster-monitor",
		Description: "Detect cluster-resource and Kubernetes event failures",
		Feature:     feature.ClusterResources,
		Resources: []string{
			"namespace", "resourcequota", "limitrange", "lease", "event",
		},
		Documentation: "/docs/general-configuration",
	}
}
