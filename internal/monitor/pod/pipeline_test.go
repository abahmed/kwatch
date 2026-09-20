package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
)

func TestBuildPodDetectorsIncludesConfiguredStages(t *testing.T) {
	cfg := &config.Config{
		Maintenance: config.MaintenanceConfig{Enabled: true},
		PendingPodMonitor: config.PendingPodMonitor{
			Enabled:   true,
			Threshold: 45,
		},
		NotReadyMonitor: config.NotReadyMonitor{Enabled: true},
	}

	detectors := BuildPodDetectorsWithRuntimeConfig(
		config.RuntimeConfigFor(cfg),
	)

	assert.Len(t, detectors, 7)
	assert.IsType(t, policy.DisruptionRule{}, detectors[0])
	assert.IsType(t, policy.NamespaceRule{}, detectors[1])
	assert.IsType(t, policy.PendingPodRule{}, detectors[5])
	assert.IsType(t, policy.NotReadyRule{}, detectors[6])
}

func TestBuildContainerDetectorsDisablesDisruptionRule(t *testing.T) {
	disabled := false
	cfg := &config.Config{IgnoreDisruptionTerminations: &disabled}

	detectors := BuildContainerDetectorsWithRuntimeConfig(
		config.RuntimeConfigFor(cfg),
	)

	assert.Len(t, detectors, 8)
	assert.IsType(t, policy.NamespaceRule{}, detectors[0])
}

func TestBuildEnrichersUseStableOrder(t *testing.T) {
	assert.Len(t, BuildPodEnrichers(), 3)
	assert.Len(t, BuildContainerSuppressionEnrichers(), 3)
	assert.Len(t, BuildContainerDataEnrichers(), 1)
}
