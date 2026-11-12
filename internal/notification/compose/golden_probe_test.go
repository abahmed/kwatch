package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// probeCases are probe failures with no deeper cause.
func probeCases() []goldenCase {
	return []goldenCase{
		{"liveness_start_error", writeLivenessStartError},
	}
}

// writeLivenessStartError: one replica of web hung at start-up after
// it logged that it could not reach its cache; its liveness probe
// killed it and its preStop hook failed as it died. Since then the pod
// was evicted and replaced, so the steps must read it through the
// Deployment, never by the old pod's name.
func writeLivenessStartError() notification.Message {
	web := inventory.CoreID(kube.KindDeployment, "shop", "web")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-6bc6ff5bd-86qv7")
	app := inventory.CoreID(kube.KindContainer, "shop",
		"web-6bc6ff5bd-86qv7/app")
	live := detection.Finding{Entity: app,
		Reason: reasons.LivenessProbeFailed, Severity: detection.Warning,
		Mode: detection.ModeProbeLiveness, Since: at(1, 0),
		Summary: "Container keeps failing its liveness probe",
		Evidence: []detection.Evidence{
			{Label: "probe", Value: "Liveness probe failed: Get " +
				"\"http://10.0.0.7:8080/live\": dial tcp 10.0.0.7:8080: " +
				"connect: connection refused"},
			{Label: detection.EvidenceProbeResult,
				Value: "connection refused"},
			{Label: "restarts", Value: "1"},
			{Label: detection.EvidenceProbeCheck, Value: "http /live :8080"},
			{Label: detection.EvidenceError, Value: "StackExchange.Redis." +
				"RedisConnectionException: It was not possible to connect " +
				"to the redis server(s). Error connecting right now."},
		}}
	hook := detection.Finding{Entity: app, Reason: "FailedPreStopHook",
		Severity: detection.Warning, Mode: "FailedPreStopHook",
		Since:   at(0, 50),
		Summary: "Kubernetes reported FailedPreStopHook"}
	notReady := detection.Finding{Entity: pod,
		Reason: reasons.ContainersNotReady, Severity: detection.Warning,
		Mode: detection.ModeNotReady, Since: at(0, 0),
		Summary: "Pod has not been ready for 3m"}
	p := incident.Incident{ID: "inc-probe", Root: web,
		Tier: incident.Notify, State: incident.Open, Opened: at(1, 0),
		Members: members(live, hook, notReady)}
	return Writer{}.Write(announce(p), at(3, 0))
}

// The steps of a liveness kill read the replaced pod through its
// Deployment and show where the probe is set; the old pod is never
// named.
func TestLivenessStartErrorStepsReadTheController(t *testing.T) {
	msg := writeLivenessStartError()
	var commands []string
	for _, s := range msg.Steps {
		commands = append(commands, s.Command)
	}
	assert.Contains(t, commands,
		"kubectl logs deploy/web -c app -n shop --previous")
	assert.Contains(t, commands, "kubectl describe deploy/web -n shop")
	for _, c := range commands {
		assert.NotContains(t, c, "web-6bc6ff5bd-86qv7")
	}
}

// How a killed container stopped never leads while the kill is there.
func TestStopEventNeverLeadsAProbeFailure(t *testing.T) {
	msg := writeLivenessStartError()
	assert.NotContains(t, msg.Title, "FailedPreStopHook")
	assert.Contains(t, msg.Title, "keeps being restarted")
}

// An earlier stop event still sorts after the failure it follows.
func TestStopEventSortsAfterOtherFailures(t *testing.T) {
	app := inventory.CoreID(kube.KindContainer, "shop", "web-1-x/app")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-1-x")
	hook := detection.Finding{Entity: app, Reason: "FailedPreStopHook",
		Severity: detection.Warning, Since: at(0, 10),
		Summary: "Kubernetes reported FailedPreStopHook"}
	notReady := detection.Finding{Entity: pod,
		Reason: reasons.ContainersNotReady, Severity: detection.Warning,
		Since: at(0, 30), Summary: "Pod has not been ready for 3m"}
	p := incident.Incident{Members: members(hook, notReady)}

	got := sortedMembers(p)

	assert.Equal(t, reasons.ContainersNotReady, got[0].Reason)
}
