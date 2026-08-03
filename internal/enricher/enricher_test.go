package enricher

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestResolveSeverityExactMatch(t *testing.T) {
	e := &DefaultEnricher{
		SeverityByOwnerKind: map[string]string{
			"StatefulSet": "high",
			"DaemonSet":   "critical",
		},
		SeverityByReason: map[string]string{
			"ImagePullBackOff": "medium",
			"Evicted":          "warning",
		},
	}
	assert.Equal(t, model.SeverityHigh, e.resolveSeverity("StatefulSet", "Error"))
	assert.Equal(t, model.SeverityCritical, e.resolveSeverity("DaemonSet", "Error"))
	assert.Equal(t, model.SeverityMedium, e.resolveSeverity("Deployment", "ImagePullBackOff"))
	assert.Equal(t, model.SeverityWarning, e.resolveSeverity("Deployment", "Evicted"))
	assert.Equal(t, model.SeverityNormal, e.resolveSeverity("Deployment", "CrashLoopBackOff"))
}

func TestResolveSeverityCaseInsensitive(t *testing.T) {
	e := &DefaultEnricher{
		SeverityByOwnerKind: map[string]string{
			"statefulset": "high",
			"daemonset":   "critical",
		},
		SeverityByReason: map[string]string{
			"imagepullbackoff": "medium",
		},
	}
	// K8s kinds/reasons arrive with canonical capitalization; config keys
	// must still resolve regardless of case.
	assert.Equal(t, model.SeverityHigh, e.resolveSeverity("StatefulSet", "Error"))
	assert.Equal(t, model.SeverityCritical, e.resolveSeverity("DaemonSet", "Error"))
	assert.Equal(t, model.SeverityMedium, e.resolveSeverity("Deployment", "ImagePullBackOff"))
}

func TestEnrichSeverityNotCorruptedByConfig(t *testing.T) {
	e := &DefaultEnricher{
		SeverityByOwnerKind: map[string]string{"StatefulSet": "high"},
	}
	inc := &model.Incident{}
	e.Enrich(&event.Event{OwnerKind: "StatefulSet"}, inc)
	assert.Equal(t, model.SeverityHigh, inc.Severity, "StatefulSet severity must come from user config")
}

func TestEnrichSeverityWarningSticky(t *testing.T) {
	e := &DefaultEnricher{
		SeverityByReason: map[string]string{"ImagePullBackOff": "normal"},
	}
	inc := &model.Incident{Severity: model.SeverityWarning}
	// A "normal" event must NOT downgrade an open "warning" incident.
	e.Enrich(&event.Event{Reason: "ImagePullBackOff"}, inc)
	assert.Equal(t, model.SeverityWarning, inc.Severity, "warning must be sticky against normal events")

	// But an explicit "high" event still escalates.
	e.Enrich(&event.Event{Severity: model.SeverityHigh}, inc)
	assert.Equal(t, model.SeverityHigh, inc.Severity)
}
