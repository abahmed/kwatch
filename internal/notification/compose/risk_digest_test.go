package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestDigestNamesWorkloadsWithPodsThatNeverBecomeReady(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	risks := []detection.Finding{
		{Advisory: true, Reason: reasons.WorkloadNeverReady,
			Entity: inventory.CoreID(kube.KindDeployment, "shop", "api")},
		{Advisory: true, Reason: reasons.WorkloadNeverReady,
			Entity: inventory.CoreID(kube.KindDeployment, "shop", "cart")},
	}

	msg := Writer{}.Digest([]incident.Decision{
		podDecision("a", incident.Digest)}, nil, risks, now)

	want := "2 workloads have pods that run but never become ready " +
		"(api, cart)"
	if !strings.Contains(msg.Note, want) {
		t.Fatalf("note = %q, want %q", msg.Note, want)
	}
}

// Pods that run but never become ready are happening now; unlike a
// configuration choice, they are listed even in a system namespace.
func TestDigestKeepsNeverReadySystemWorkloads(t *testing.T) {
	risks := []detection.Finding{
		{Advisory: true, Reason: reasons.WorkloadNeverReady,
			Entity: inventory.CoreID(kube.KindDeployment, "kube-system",
				"coredns")},
		{Advisory: true, Reason: reasons.RiskSingleReplica,
			Entity: inventory.CoreID(kube.KindDeployment, "kube-system",
				"coredns")},
	}

	kept := withoutSystemRisks(risks)

	if len(kept) != 1 || kept[0].Reason != reasons.WorkloadNeverReady {
		t.Fatalf("kept = %+v", kept)
	}
}
