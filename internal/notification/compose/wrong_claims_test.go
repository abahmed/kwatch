package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A node that was only under pressure never stopped reporting: when the
// members are gone at the resolve, the cause's own findings still say
// what ended.
func TestNodeResolveKeepsThePressureThatEnded(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n1")
	for _, c := range []struct {
		name  string
		cause *rootcause.CauseRecord
		want  string
	}{
		{"memory threshold", &rootcause.CauseRecord{Root: node,
			Rule: "node-memory-pressure",
			RootFindings: []detection.Finding{{Entity: node,
				Reason: reasons.EvictionThresholdMet,
				Mode:   detection.ModeMemoryEvictionThreshold}}},
			"is no longer low on memory"},
		{"disk threshold", &rootcause.CauseRecord{Root: node,
			Rule: "node-disk-pressure",
			RootFindings: []detection.Finding{{Entity: node,
				Reason: reasons.EvictionThresholdMet,
				Mode:   detection.ModeDiskEvictionThreshold}}},
			"is no longer low on disk space"},
		{"not ready", &rootcause.CauseRecord{Root: node,
			Rule: "node-not-ready",
			RootFindings: []detection.Finding{{Entity: node,
				Reason: reasons.NodeNotReady}}},
			"is ready again"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := incident.Incident{ID: "inc-n", Root: node,
				State: incident.Resolved, Opened: at(0, 0),
				Resolved: at(9, 0), Cause: c.cause}

			_, got := resolveNote(caseFacts{p: p, now: at(10, 0),
				reason: "healthy"})

			assert.Contains(t, got[0].text, c.want)
		})
	}
}

// evictedWithRisk is a workload whose pod was evicted, with the
// workload's own no-memory-limit risk beside it.
func evictedWithRisk(message string) incident.Incident {
	app := inventory.CoreID(kube.KindDeployment, "render", "thumbnailer")
	pod := inventory.CoreID(kube.KindPod, "render", "thumbnailer-7d9f-a")
	return incident.Incident{ID: "inc-1", Root: app, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Members: members(
			detection.Finding{Entity: pod, Reason: reasons.Evicted,
				Mode: detection.ModeEvicted, Severity: detection.Warning,
				Since: at(0, 0), Summary: "Pod was evicted",
				Evidence: []detection.Evidence{
					{Label: "message", Value: message}}},
			detection.Finding{Entity: app, Reason: reasons.RiskNoMemoryLimit,
				Mode: detection.ModeRiskNoMemoryLimit, Advisory: true,
				Severity: detection.Info, Since: at(0, 0),
				Summary: "no memory limit"})}
}

func TestRiskNoMemoryLimitIsOnlyForMemoryEvictions(t *testing.T) {
	disk := Writer{}.Write(announce(evictedWithRisk(
		"The node was low on resource: ephemeral-storage. Threshold "+
			"quantity: 1Gi, available: 800Mi.")), at(6, 0)).Note
	memory := Writer{}.Write(announce(evictedWithRisk(
		"The node was low on resource: memory. Threshold quantity: "+
			"100Mi, available: 81Mi.")), at(6, 0)).Note

	assert.NotContains(t, disk, "no memory limit")
	assert.Contains(t, memory, "have no memory limit")
}

// A cause that is one of the lead workload's own pods says nothing the
// lead does not: "ccm keeps crashing because its own pod is not ready".
func TestLeadDropsACauseThatIsItsOwnPod(t *testing.T) {
	app := inventory.CoreID(kube.KindDeployment, "kube-system", "ccm")
	pod := inventory.CoreID(kube.KindPod, "kube-system", "ccm-e465b17e-a")
	ct := inventory.CoreID(kube.KindContainer, "kube-system",
		"ccm-e465b17e-a/app")
	notReady := detection.Finding{Entity: pod,
		Reason: reasons.ContainersNotReady, Mode: detection.ModeNotReady,
		Severity: detection.Warning, Since: at(0, 0),
		Summary: "Pod has not been ready for three minutes"}
	p := incident.Incident{ID: "inc-1", Root: app, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Impact: []inventory.EntityID{app},
		Members: members(notReady,
			detection.Finding{Entity: ct, Reason: reasons.CrashLoopBackOff,
				Mode: detection.ModeCrashLoop, Severity: detection.Critical,
				Since: at(0, 0), Summary: "Container is crash looping"}),
		Cause: &rootcause.CauseRecord{Rule: "lease-holder-failing",
			Root: pod, RootFindings: []detection.Finding{notReady},
			Chain: []inventory.EntityID{pod, app}, Score: 0.8}}

	msg := Writer{}.Write(announce(p), at(6, 0))

	assert.Equal(t, "ccm in kube-system is crash looping.", msg.Title)
	assert.NotContains(t, msg.Note, "because pod")
}
