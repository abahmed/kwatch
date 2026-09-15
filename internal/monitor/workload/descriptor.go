package workload

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Descriptor returns the stable metadata for the workload monitor family.
func Descriptor() monitor.Descriptor {
	return monitor.Descriptor{
		Name:        "workload-monitor",
		Description: "Detect workload rollout and execution failures",
		Feature:     feature.WorkloadDetection,
		Resources: []string{
			"deployment",
			"statefulset",
			"daemonset",
			"replicaset",
			"job",
			"cronjob",
			"poddisruptionbudget",
			"horizontalpodautoscaler",
		},
		Documentation: "/docs/general-configuration",
	}
}
