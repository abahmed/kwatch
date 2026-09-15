package pod

import (
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
)

// BuildPodDetectorsWithRuntimeConfig assembles the immutable runtime policy.
func BuildPodDetectorsWithRuntimeConfig(
	runtime config.RuntimeConfig,
) []policy.Detector {
	detectors := []policy.Detector{
		policy.NamespaceRule{},
		policy.PodNameRule{},
		policy.PodStatusRule{},
	}
	if runtime.Maintenance().Enabled {
		detectors = append(detectors, policy.MaintenanceRule{})
	}

	if runtime.PendingPodMonitor().Enabled {
		threshold := time.Duration(
			runtime.PendingPodMonitor().Threshold,
		) * time.Second
		if threshold <= 0 {
			threshold = 300 * time.Second
		}
		detectors = append(
			detectors,
			policy.PendingPodRule{Threshold: threshold},
		)
	}

	if runtime.NotReadyMonitor().Enabled {
		detectors = append(
			detectors,
			policy.NotReadyRule{
				Threshold: policy.DefaultNotReadyThreshold,
			},
		)
	}

	if runtime.IgnoreDisruptionTerminations() {
		detectors = prependDisruptionRule(detectors)
	}
	return detectors
}

// BuildPodEnrichers returns the ordered pod enrichment chain.
func BuildPodEnrichers() []enrichment.Enricher {
	return []enrichment.Enricher{
		enrichment.PodEventsEnricher{},
		enrichment.EventMessageEnricher{},
		enrichment.PodOwnersEnricher{},
	}
}

// BuildContainerDetectorsWithRuntimeConfig assembles container policy from
// the immutable runtime snapshot.
func BuildContainerDetectorsWithRuntimeConfig(
	runtime config.RuntimeConfig,
) []policy.Detector {
	detectors := []policy.Detector{
		policy.NamespaceRule{},
		policy.PodNameRule{},
		policy.ContainerNameRule{},
		policy.ContainerRestartsRule{},
		policy.ContainerStateRule{},
		policy.ContainerReasonsRule{},
		policy.NoiseRule{},
		policy.ContainerMessageRule{},
	}
	if runtime.Maintenance().Enabled {
		detectors = append(detectors, policy.MaintenanceRule{})
	}

	if runtime.IgnoreDisruptionTerminations() {
		detectors = prependDisruptionRule(detectors)
	}
	return detectors
}

// BuildContainerSuppressionEnrichers returns enrichers that can suppress a
// container finding after its state has been detected.
func BuildContainerSuppressionEnrichers() []enrichment.Enricher {
	return []enrichment.Enricher{
		enrichment.ContainerKillingEnricher{},
		enrichment.EventMessageEnricher{},
		enrichment.ContainerLogsEnricher{},
	}
}

// BuildContainerDataEnrichers returns enrichers that add data even when a
// suppression enricher has decided not to announce the current finding.
func BuildContainerDataEnrichers() []enrichment.Enricher {
	return []enrichment.Enricher{enrichment.PodOwnersEnricher{}}
}

func prependDisruptionRule(
	detectors []policy.Detector,
) []policy.Detector {
	return append([]policy.Detector{policy.DisruptionRule{}}, detectors...)
}
