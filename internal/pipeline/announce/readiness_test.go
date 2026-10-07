package announce

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/status"
)

func blockers(names ...string) func() status.Readiness {
	return func() status.Readiness {
		var findings []detection.Finding
		for _, n := range names {
			findings = append(findings, detection.Finding{
				Entity: inventory.CoreID(kube.KindNode, "", n),
				Reason: reasons.ClusterVersionSkew, Summary: "Kubelet old"})
		}
		return status.Assess(findings, nil)
	}
}

func TestReadinessLineRidesTheDigestOncePerDayAndOnlyOnChange(t *testing.T) {
	now := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	var item readinessItem
	item.source = blockers("n1")
	g := &LowDigest{}

	first := notification.Message{Note: "digest."}
	item.attach(&first, g, now)
	if !strings.Contains(first.Note, "Upgrade readiness: 1 blocker") ||
		len(first.Lines) != 1 || len(first.Doc) != 1 {
		t.Fatalf("first = %+v", first)
	}

	same := notification.Message{}
	item.attach(&same, g, now.Add(48*time.Hour))
	if same.Note != "" {
		t.Fatal("an unchanged answer is not repeated")
	}

	item.source = blockers("n1", "n2")
	soon := notification.Message{}
	item.attach(&soon, g, now.Add(2*time.Hour))
	if soon.Note != "" {
		t.Fatal("at most once a day")
	}
	item.attach(&soon, g, now.Add(25*time.Hour))
	if !strings.Contains(soon.Note, "2 blockers") {
		t.Fatalf("a changed answer after a day is sent: %q", soon.Note)
	}
}

func TestReadinessWithoutSourceOrBlockersAddsNothing(t *testing.T) {
	var item readinessItem
	msg := notification.Message{}
	item.attach(&msg, &LowDigest{}, time.Now())
	item.source = blockers()
	item.attach(&msg, &LowDigest{}, time.Now())
	if msg.Note != "" || len(msg.Doc) != 0 {
		t.Fatalf("msg = %+v", msg)
	}
}
