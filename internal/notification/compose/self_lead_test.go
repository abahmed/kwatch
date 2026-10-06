package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A failure that has no cause outside itself must not be told as
// "X is failing because it is failing on its own".
func TestSelfCauseLeadIsOnlyTheState(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	root := inventory.CoreID(kube.KindDeployment, "staging", "assets")
	pod := inventory.CoreID(kube.KindPod, "staging", "assets-1")
	members := map[detection.Key]detection.Finding{
		{Entity: pod, Reason: "ContainersNotReady"}: {Entity: pod,
			Reason: "ContainersNotReady", Severity: detection.Critical,
			Summary: "Pod has not been ready for three minutes"},
	}
	p := incident.Incident{
		ID: "inc-1", Root: root, Tier: incident.Notify,
		State: incident.Open, Opened: now, Announced: now, Revision: 1,
		Members: members,
		Cause:   &rootcause.CauseRecord{Root: root, Rule: "self"},
	}

	msg := Writer{}.Write(incident.Decision{Action: incident.Announce,
		Incident: p}, now)

	for _, bad := range []string{"because", "on its own"} {
		if strings.Contains(msg.Title, bad) {
			t.Fatalf("title %q must not contain %q", msg.Title, bad)
		}
	}
	if !strings.Contains(msg.Title, "assets") {
		t.Fatalf("title = %q, want the subject", msg.Title)
	}
}
