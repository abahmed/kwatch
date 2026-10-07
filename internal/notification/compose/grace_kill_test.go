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

func killedPod(name string, rollout bool, more ...detection.Evidence,
) detection.Finding {
	at := time.Date(2026, 3, 1, 10, 2, 0, 0, time.UTC)
	evidence := append([]detection.Evidence{
		{Label: detection.EvidenceGracePeriod, Value: "30s"},
		{Label: detection.EvidenceKilledContainers, Value: "app"},
	}, more...)
	if rollout {
		evidence = append(evidence, detection.Evidence{
			Label: detection.EvidenceDuringRollout, Value: "true"})
	}
	return detection.Finding{
		Entity: inventory.CoreID(kube.KindPod, "shop", name),
		Reason: reasons.PodKilledAtGrace, Severity: detection.Warning,
		Since:    at,
		Summary:  "Pod did not stop within its 30s grace period",
		Evidence: evidence,
	}
}

func graceNote(killed ...detection.Finding) string {
	root := inventory.CoreID(kube.KindDeployment, "shop", "api")
	return Writer{}.Write(announce(incident.Incident{
		ID: "grace-1", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: writerNow.Add(-time.Hour),
		Members: members(killed...),
	}), writerNow).Note
}

func TestWriteGraceKillNamesTheRolloutAndTheGracePeriod(t *testing.T) {
	got := graceNote(killedPod("api-a", true), killedPod("api-b", true),
		killedPod("api-c", true))

	assert.Contains(t, got, "During the rollout at 10:02, 3 api pods did "+
		"not stop within their 30s grace period and were killed (exit "+
		"code 137); work in flight was cut off.")
}

func TestWriteGraceKillOutsideARolloutDoesNotSayRollout(t *testing.T) {
	got := graceNote(killedPod("api-a", false))

	assert.NotContains(t, got, "rollout")
	assert.Contains(t, got, "At 10:02, 1 api pod did not stop within its "+
		"30s grace period and was killed (exit code 137); work in "+
		"flight was cut off.")
}

func TestWriteGraceKillQuotesTheKubeletsHookFailure(t *testing.T) {
	got := graceNote(killedPod("api-a", true, detection.Evidence{
		Label: detection.EvidenceStopHook,
		Value: "Exec lifecycle hook ([sleep 60]) failed"}))

	assert.Contains(t, got, "The kubelet reported \"Exec lifecycle hook "+
		"([sleep 60]) failed\".")
}
