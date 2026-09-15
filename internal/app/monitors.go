package app

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/monitor/cluster"
	"github.com/abahmed/kwatch/internal/monitor/network"
	"github.com/abahmed/kwatch/internal/monitor/node"
	"github.com/abahmed/kwatch/internal/monitor/pod"
	"github.com/abahmed/kwatch/internal/monitor/security"
	"github.com/abahmed/kwatch/internal/monitor/workload"
)

// newMonitorRegistry validates the user-visible monitor vocabulary at the
// composition root. It describes capabilities only; runtime construction
// remains explicit in runtime_build.go and runtime_optional.go.
func newMonitorRegistry() (*monitor.Registry, error) {
	return monitor.NewRegistry(
		pod.Descriptor(),
		workload.Descriptor(),
		node.Descriptor(),
		cluster.Descriptor(),
		monitor.Descriptor{
			Name:          "storage-monitor",
			Description:   "Detect persistent storage failures",
			Feature:       feature.StorageDetection,
			Resources:     []string{"persistentvolumeclaim", "persistentvolume"},
			Documentation: "/docs/general-configuration",
		},
		network.Descriptor(),
		security.Descriptor(),
		monitor.Descriptor{
			Name:          "control-plane-monitor",
			Description:   "Detect control-plane health failures",
			Feature:       feature.ControlPlanePods,
			Resources:     []string{"pod", "kubernetes-api"},
			Documentation: "/docs/general-configuration",
		},
		monitor.Descriptor{
			Name:          "telemetry-monitor",
			Description:   "Read kubelet and metrics API telemetry",
			Feature:       feature.KubeletTelemetry,
			Resources:     []string{"node", "pod"},
			Documentation: "/docs/general-configuration",
		},
		monitor.Descriptor{
			Name:          "probe-monitor",
			Description:   "Run configured active endpoint probes",
			Feature:       feature.HTTPProbes,
			Resources:     []string{"http", "tcp", "dns"},
			Documentation: "/docs/general-configuration",
		},
	)
}
