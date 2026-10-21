package v1beta1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestSilenceRuleDeepCopyIsNotAliased(t *testing.T) {
	in := &SilenceRule{
		Namespaces:        []string{"ns1"},
		Reasons:           []string{"OOMKilled"},
		PodNamePatterns:   []string{"^app-"},
		ContainerNames:    []string{"sidecar"},
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
	out.ContainerMessages[0] = "mutated"
	out.EventMessages[0] = "mutated"
	out.NodeReasons[0] = "mutated"
	out.NodeMessages[0] = "mutated"

	assert.Equal(t, []string{"ns1"}, in.Namespaces)
	assert.Equal(t, []string{"OOMKilled"}, in.Reasons)
	assert.Equal(t, []string{"^app-"}, in.PodNamePatterns)
	assert.Equal(t, []string{"sidecar"}, in.ContainerNames)
	assert.Equal(t, []string{"denied"}, in.ContainerMessages)
	assert.Equal(t, []string{"cache timed out"}, in.EventMessages)
	assert.Equal(t, []string{"KubeletNotReady"}, in.NodeReasons)
	assert.Equal(t, []string{"draining"}, in.NodeMessages)
}

func TestKwatchConfigSpecDeepCopyCopiesMonitorFields(t *testing.T) {
	in := &KwatchConfigSpec{
		Maintenance: MaintenanceConfig{
			Enabled:         ptr.To(true),
			Annotation:      "kwatch.io/maintenance",
			UntilAnnotation: "kwatch.io/maintenance-until",
		},
		Telemetry: TelemetryConfig{Enabled: ptr.To(true)},
		ActiveProbeMonitor: MonitorConfig{
			"http": []interface{}{"https://example.test"},
		},
	}
	out := in.DeepCopy()
	out.ActiveProbeMonitor["http"].([]interface{})[0] = "changed"

	assert.Equal(t, in.Maintenance, out.Maintenance)
	assert.Equal(t, in.Telemetry, out.Telemetry)
	assert.Equal(
		t,
		"https://example.test",
		in.ActiveProbeMonitor["http"].([]interface{})[0],
	)
}

func TestKwatchConfigSpecDeepCopyDetachesMutableFields(t *testing.T) {
	in := &KwatchConfigSpec{
		Crd:              MonitorConfig{"enabled": true},
		Upgrader:         map[string]interface{}{"disableUpdateCheck": true},
		SeverityByReason: map[string]string{"OOMKilled": "critical"},
		Runbooks:         map[string]string{"OOMKilled": "https://runbook"},
	}

	out := in.DeepCopy()
	out.Crd["enabled"] = false
	out.Upgrader["disableUpdateCheck"] = false
	out.SeverityByReason["OOMKilled"] = "warning"
	out.Runbooks["OOMKilled"] = "changed"

	assert.Equal(t, true, in.Crd["enabled"])
	assert.Equal(t, true, in.Upgrader["disableUpdateCheck"])
	assert.Equal(t, "critical", in.SeverityByReason["OOMKilled"])
	assert.Equal(t, "https://runbook", in.Runbooks["OOMKilled"])
}

// A switch set to false must survive encoding: a plain bool with omitempty
// drops it, and the object reads as if the switch were never set.
func TestSwitchesEncodeAnExplicitFalse(t *testing.T) {
	spec := KwatchConfigSpec{
		Telemetry:        TelemetryConfig{Enabled: ptr.To(false)},
		Maintenance:      MaintenanceConfig{Enabled: ptr.To(false)},
		AuditLog:         AuditLogConfig{Enabled: ptr.To(false)},
		HeartbeatMonitor: HeartbeatMonitorConfig{Enabled: ptr.To(false)},
		App:              AppConfig{DisableStartupMessage: ptr.To(false)},
	}

	raw, err := json.Marshal(spec)
	require.NoError(t, err)

	var back map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, false, back["telemetry"]["enabled"])
	assert.Equal(t, false, back["maintenance"]["enabled"])
	assert.Equal(t, false, back["auditLog"]["enabled"])
	assert.Equal(t, false, back["heartbeatMonitor"]["enabled"])
	assert.Equal(t, false, back["app"]["disableStartupMessage"])

	unset, err := json.Marshal(KwatchConfigSpec{})
	require.NoError(t, err)
	assert.NotContains(t, string(unset), "enabled", "unset stays absent")
}

func TestSwitchPointersAreNotAliasedByDeepCopy(t *testing.T) {
	in := &KwatchConfigSpec{
		Telemetry:        TelemetryConfig{Enabled: ptr.To(true)},
		Maintenance:      MaintenanceConfig{Enabled: ptr.To(true)},
		AuditLog:         AuditLogConfig{Enabled: ptr.To(true)},
		HeartbeatMonitor: HeartbeatMonitorConfig{Enabled: ptr.To(true)},
		App:              AppConfig{DisableStartupMessage: ptr.To(true)},
	}

	out := in.DeepCopy()
	*out.Telemetry.Enabled = false
	*out.Maintenance.Enabled = false
	*out.AuditLog.Enabled = false
	*out.HeartbeatMonitor.Enabled = false
	*out.App.DisableStartupMessage = false

	assert.True(t, *in.Telemetry.Enabled)
	assert.True(t, *in.Maintenance.Enabled)
	assert.True(t, *in.AuditLog.Enabled)
	assert.True(t, *in.HeartbeatMonitor.Enabled)
	assert.True(t, *in.App.DisableStartupMessage)
}
