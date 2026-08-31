package handler

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/filter"
)

// buildPodDetectors assembles the ordered pod-level detector chain from
// configuration. Detectors run in order; the first Skip verdict drops the pod.
func buildPodDetectors(cfg *config.Config) []filter.Detector {
	podDetectors := []filter.Detector{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.PodStatusFilter{},
	}
	if cfg.Maintenance.Enabled {
		podDetectors = append(podDetectors, filter.MaintenanceFilter{})
	}

	if cfg.PendingPodMonitor.Enabled {
		pendingThreshold := time.Duration(cfg.PendingPodMonitor.Threshold) * time.Second
		if pendingThreshold <= 0 {
			pendingThreshold = 300 * time.Second
		}
		podDetectors = append(podDetectors, filter.PendingPodFilter{Threshold: pendingThreshold})
	}

	if cfg.NotReadyMonitor.Enabled {
		podDetectors = append(podDetectors, filter.NotReadyFilter{Threshold: filter.DefaultNotReadyThreshold})
	}

	if cfg.IgnoreDisruptionTerminations == nil || *cfg.IgnoreDisruptionTerminations {
		podDetectors = prependDisruptionFilter(podDetectors)
	}
	return podDetectors
}

func buildPodEnrichers() []filter.Enricher {
	return []filter.Enricher{
		filter.PodEventsFilter{},
		filter.PodOwnersFilter{},
	}
}

// buildContainerDetectors assembles the ordered container-level detector chain.
func buildContainerDetectors(cfg *config.Config) []filter.Detector {
	containerDetectors := []filter.Detector{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.ContainerNameFilter{},
		filter.ContainerRestartsFilter{},
		filter.ContainerStateFilter{},
		filter.ContainerReasonsFilter{},
		filter.NoiseFilter{},
		filter.ContainerMessageFilter{},
	}
	if cfg.Maintenance.Enabled {
		containerDetectors = append(containerDetectors, filter.MaintenanceFilter{})
	}

	if cfg.IgnoreDisruptionTerminations == nil || *cfg.IgnoreDisruptionTerminations {
		containerDetectors = prependDisruptionFilter(containerDetectors)
	}
	return containerDetectors
}

func buildContainerSuppressionEnrichers() []filter.Enricher {
	return []filter.Enricher{
		filter.ContainerKillingFilter{},
		filter.ContainerLogsFilter{},
	}
}

func buildContainerDataEnrichers() []filter.Enricher {
	return []filter.Enricher{
		filter.PodOwnersFilter{},
	}
}

func prependDisruptionFilter(ds []filter.Detector) []filter.Detector {
	return append([]filter.Detector{filter.DisruptionFilter{}}, ds...)
}
