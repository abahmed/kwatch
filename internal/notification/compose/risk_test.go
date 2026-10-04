package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// A risk on the failing workload is quoted as the cost of the failure,
// and never leads the message.
func TestRiskSentencesNameTheCostOfARisk(t *testing.T) {
	orders := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	ct := inventory.CoreID(kube.KindContainer, "shop", "orders-1/app")
	crash := detection.Finding{Entity: ct, Reason: reasons.LivenessProbeFailed,
		Mode: detection.ModeProbeLiveness, Severity: detection.Warning,
		Since: at(3, 0), Summary: "Container keeps failing its liveness probe"}
	risk := detection.Finding{Entity: orders,
		Reason: reasons.RiskNoReadinessProbe,
		Mode:   detection.ModeRiskNoReadinessProbe, Severity: detection.Info,
		Advisory: true, Since: at(1, 0),
		Summary: "Its containers have no readiness probe"}
	p := incident.Incident{ID: "inc-1", Root: orders, Tier: incident.Notify,
		State: incident.Open, Opened: at(3, 0), Revision: 1,
		Members: members(crash, risk)}

	msg := Writer{}.Write(announce(p), at(6, 0))
	text := notification.Text(msg)

	if strings.Contains(msg.Title, "readiness probe") {
		t.Fatalf("the failure leads, not the risk: %q", msg.Title)
	}
	if !strings.Contains(text, "It has no readiness probe, so traffic "+
		"kept reaching it while it failed.") {
		t.Fatalf("note = %s", text)
	}
}

// A risk the failure does not touch is not mentioned.
func TestRiskSentencesSkipUnrelatedRisks(t *testing.T) {
	orders := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	ct := inventory.CoreID(kube.KindContainer, "shop", "orders-1/app")
	crash := detection.Finding{Entity: ct, Reason: reasons.CrashLoopBackOff,
		Mode: detection.ModeCrashLoop, Severity: detection.Critical,
		Since: at(3, 0), Summary: "Container is crash looping"}
	risk := detection.Finding{Entity: orders, Reason: reasons.RiskNoMemoryLimit,
		Mode: detection.ModeRiskNoMemoryLimit, Severity: detection.Info,
		Advisory: true, Since: at(1, 0), Summary: "no memory limit"}
	p := incident.Incident{ID: "inc-1", Root: orders, Tier: incident.Notify,
		State: incident.Open, Opened: at(3, 0), Revision: 1,
		Members: members(crash, risk)}

	text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

	if strings.Contains(text, "memory limit") {
		t.Fatalf("an unrelated risk must stay in the digest: %s", text)
	}
}
