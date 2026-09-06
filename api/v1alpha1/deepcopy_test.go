package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSilenceRuleDeepCopyIsNotAliased(t *testing.T) {
	in := &SilenceRule{
		Namespaces:        []string{"ns1"},
		Reasons:           []string{"OOMKilled"},
		PodNamePatterns:   []string{"^app-"},
		ContainerNames:    []string{"sidecar"},
		LogPatterns:       []string{"fatal"},
		ContainerMessages: []string{"denied"},
		EventMessages:     []string{"cache timed out"},
		NodeReasons:       []string{"KubeletNotReady"},
		NodeMessages:      []string{"draining"},
	}

	out := in.DeepCopy()

	// Mutating the copy must not affect the original (no aliased slices).
	out.Namespaces[0] = "mutated"
	out.Reasons[0] = "mutated"
	out.PodNamePatterns[0] = "mutated"
	out.ContainerNames[0] = "mutated"
	out.LogPatterns[0] = "mutated"
	out.ContainerMessages[0] = "mutated"
	out.EventMessages[0] = "mutated"
	out.NodeReasons[0] = "mutated"
	out.NodeMessages[0] = "mutated"

	assert.Equal(t, []string{"ns1"}, in.Namespaces)
	assert.Equal(t, []string{"OOMKilled"}, in.Reasons)
	assert.Equal(t, []string{"^app-"}, in.PodNamePatterns)
	assert.Equal(t, []string{"sidecar"}, in.ContainerNames)
	assert.Equal(t, []string{"fatal"}, in.LogPatterns)
	assert.Equal(t, []string{"denied"}, in.ContainerMessages)
	assert.Equal(t, []string{"cache timed out"}, in.EventMessages)
	assert.Equal(t, []string{"KubeletNotReady"}, in.NodeReasons)
	assert.Equal(t, []string{"draining"}, in.NodeMessages)
}

func TestKwatchConfigSpecDeepCopyCopiesMonitorFields(t *testing.T) {
	in := &KwatchConfigSpec{
		AdaptiveThresholds: true,
		Maintenance: MaintenanceConfig{
			Enabled:         true,
			Annotation:      "kwatch.io/maintenance",
			UntilAnnotation: "kwatch.io/maintenance-until",
		},
		Telemetry: TelemetryConfig{Enabled: true},
		ActiveProbeMonitor: MonitorConfig{
			"http": []interface{}{"https://example.test"},
		},
	}
	out := in.DeepCopy()
	out.ActiveProbeMonitor["http"].([]interface{})[0] = "changed"

	assert.True(t, out.AdaptiveThresholds)
	assert.Equal(t, in.Maintenance, out.Maintenance)
	assert.Equal(t, in.Telemetry, out.Telemetry)
	assert.Equal(
		t,
		"https://example.test",
		in.ActiveProbeMonitor["http"].([]interface{})[0],
	)
}
