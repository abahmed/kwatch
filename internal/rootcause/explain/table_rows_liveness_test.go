package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// liveKill makes the pods of workload fail as liveness kills; evidence
// is what the detector wrote beside the kill.
func liveKill(
	f *fixture, pods []inventory.EntityID, evidence ...detection.Evidence,
) {
	for _, pod := range pods {
		id := containerOf(pod)
		f.fail(id, "CrashLoop.Liveness", failingH, 2,
			"Liveness probe failed: connection refused")
		last := &f.findings[id][len(f.findings[id])-1]
		last.Evidence = append(last.Evidence, evidence...)
	}
}

var (
	livenessGives = detection.Evidence{
		Label: detection.EvidenceLivenessGives,
		Value: "40s (10s delay + 3 × 10s)"}
	usualStart = detection.Evidence{
		Label: detection.EvidenceUsualStart, Value: "75s"}
	neverReady = detection.Evidence{
		Label: detection.EvidenceKilledBeforeReady, Value: "true"}
)

// livenessRowCases are the fixtures of the liveness rows: the start
// budget rows and the cascade, which reuses the called-service rows.
var livenessRowCases = append(append([]rowCase(nil), livenessStartCases...),
	cascadeRowCases...)

var livenessStartCases = []rowCase{
	{row: "liveness-shorter-than-start", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "api", 3)
			liveKill(f, pods, livenessGives, usualStart)
			return containerOf(pods[0])
		}},
	{row: "liveness-kills-before-ready", want: "deployment/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "api", 3)
			liveKill(f, pods, livenessGives, neverReady)
			return containerOf(pods[0])
		}},
}

// TestLivenessStartNeedsTheDetectorsEvidence: a liveness kill loop
// without the start evidence is not blamed on the probe's budget.
func TestLivenessStartNeedsTheDetectorsEvidence(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 3)
	liveKill(f, pods)
	if c, ok := f.explain().CauseOf(containerOf(pods[0])); ok &&
		(c.Row == "liveness-shorter-than-start" ||
			c.Row == "liveness-kills-before-ready") {
		t.Fatalf("blamed the liveness budget without evidence: %s", c.Row)
	}
}

// TestLivenessHistoryOutranksTheBareFact: when both could apply, the
// start history is the stronger proof.
func TestLivenessHistoryOutranksTheBareFact(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 3)
	liveKill(f, pods, livenessGives, usualStart, neverReady)
	c := requireCause(t, f.explain(), containerOf(pods[0]),
		"deployment/shop/api")
	if c.Row != "liveness-shorter-than-start" {
		t.Fatalf("row = %s", c.Row)
	}
}
