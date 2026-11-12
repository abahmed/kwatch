package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeIncident is web in shop whose container's probe of reason failed
// with evidence.
func probeIncident(
	reason string, ev ...detection.Evidence,
) incident.Incident {
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "web")
	container := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "shop", Name: "web-1-x/app"}
	return incident.Incident{
		ID: "probe-1", Root: deploy, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-10 * time.Minute),
		Members: members(detection.Finding{Entity: container,
			Reason: reason, Severity: detection.Warning,
			Since:    writerNow.Add(-10 * time.Minute),
			Summary:  "Container app keeps failing its probe",
			Evidence: ev}),
	}
}

func TestProbeLeadSaysWhatTheCheckAnswers(t *testing.T) {
	check := detection.Evidence{Label: detection.EvidenceProbeCheck,
		Value: "http /readyz :8081"}
	cases := []struct {
		name, reason string
		ev           []detection.Evidence
		want         string
	}{
		{"readiness answers an error", reasons.ReadinessProbeFailed,
			[]detection.Evidence{check, {
				Label: detection.EvidenceProbeResult, Value: "HTTP 500"}},
			"web in shop is not ready: its readiness check (/readyz " +
				"on port 8081) answers HTTP 500"},
		{"with the app's own words", reasons.ReadinessProbeFailed,
			[]detection.Evidence{check,
				{Label: detection.EvidenceProbeResult, Value: "HTTP 503"},
				{Label: detection.EvidenceProbeLog,
					Value: "health check failed: cache cold"}},
			"is not ready: its readiness check (/readyz on port 8081) " +
				"answers HTTP 503"},
		{"liveness times out", reasons.LivenessProbeFailed,
			[]detection.Evidence{check,
				{Label: detection.EvidenceProbeResult, Value: "timed out"},
				{Label: detection.EvidenceProbeTimeout, Value: "2"}},
			"web in shop keeps being restarted: its liveness check " +
				"(/readyz on port 8081) does not answer within 2s"},
		{"liveness refused after a start-up error",
			reasons.LivenessProbeFailed,
			[]detection.Evidence{check,
				{Label: detection.EvidenceProbeResult,
					Value: "connection refused"},
				{Label: detection.EvidenceError, Value: "java.net." +
					"ConnectException: Connection refused to cache:6379"}},
			"web in shop keeps being restarted: its liveness check " +
				"(/readyz on port 8081) fails (connection refused) after " +
				"it failed at start-up: ConnectException: Connection " +
				"refused to cache:6379"},
		{"readiness fails without a known result",
			reasons.ReadinessProbeFailed, []detection.Evidence{check},
			"web in shop is not ready: its readiness check (/readyz on " +
				"port 8081) fails"},
	}
	for _, c := range cases {
		msg := Writer{}.Write(announce(probeIncident(c.reason, c.ev...)),
			writerNow)
		assert.Contains(t, msg.Title, c.want, c.name)
	}
}

// A refused connection with nothing else known is still said plainly.
func TestProbeLeadSaysARefusedConnectionPlainly(t *testing.T) {
	p := probeIncident(reasons.ReadinessProbeFailed,
		detection.Evidence{Label: detection.EvidenceProbeResult,
			Value: "connection refused"})
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "web in shop is not ready: its "+
		"readiness check fails (connection refused)")
}

// A liveness kill leads before the readiness failure it causes.
func TestProbeLeadPrefersTheLivenessKill(t *testing.T) {
	p := probeIncident(reasons.ReadinessProbeFailed)
	container := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "shop", Name: "web-1-x/app"}
	live := detection.Finding{Entity: container,
		Reason: reasons.LivenessProbeFailed, Severity: detection.Warning,
		Since: writerNow.Add(-9 * time.Minute)}
	p.Members[live.Key()] = live
	msg := Writer{}.Write(announce(p), writerNow)
	assert.Contains(t, msg.Title, "web in shop keeps being restarted: "+
		"its liveness check fails")
}

func TestProbeTarget(t *testing.T) {
	assert.Equal(t, " (/healthz on port 8080)",
		probeTarget("http /healthz :8080"))
	assert.Equal(t, " (port 5432)", probeTarget("tcp :5432"))
	assert.Equal(t, " (gRPC on port 9090)", probeTarget("grpc :9090"))
	assert.Equal(t, "", probeTarget("exec"))
	assert.Equal(t, "", probeTarget(""))
}

// A container that exits on its own is not killed by its probe: the
// probe lead stays out and the exit leads.
func TestProbeLeadLeavesAContainerThatExitsOnItsOwn(t *testing.T) {
	p := probeIncident(reasons.LivenessProbeFailed,
		detection.Evidence{Label: detection.EvidenceProbeResult,
			Value: "connection refused"},
		detection.Evidence{Label: detection.EvidenceExitedOnOwn,
			Value: "1"})
	msg := Writer{}.Write(announce(p), writerNow)
	assert.NotContains(t, msg.Title, "keeps being restarted")
}
