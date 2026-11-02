package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A carried follower that only a listing announced speaks of what the
// listing named, like a resolve sent on its own, and names the root it
// ended on as the last one affected.
func TestEngineJointFollowerSpeaksAsAnnounced(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	moved := resolved("b", true)
	moved.Incident.Subject = inventory.EntityID{Kind: kube.KindDeployment,
		Namespace: "shop", Name: "listed"}
	e.announcer.collect.Startup.Summary.Incidents = []string{"b"}

	e.announcer.announce(context.Background(), now, []incident.Decision{
		resolved("a", true), moved})

	for _, m := range *seen {
		if m.Carrier != "" {
			continue
		}
		if !strings.Contains(m.Note, "listed in shop") ||
			!strings.Contains(m.Note, "Last affected: deployment") {
			t.Fatalf("the follower must name its listed subject: %q",
				m.Note)
		}
		return
	}
	t.Fatalf("no joint message: %+v", *seen)
}
