package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func outageCases(n int, opened time.Time) []incident.Decision {
	var out []incident.Decision
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"}[:n] {
		d := orderCase(name, incident.Notify)
		d.Incident.Opened = opened
		out = append(out, d)
	}
	return out
}

func TestNamespaceOutageLeadCountsAndDatesTheFailures(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	ds := outageCases(5, time.Date(2026, 9, 29, 10, 2, 0, 0, time.UTC))

	msg := Writer{}.NamespaceOutage(
		NamespaceOutage{Namespace: "shop", Workloads: 15}, ds, now)

	if !strings.HasPrefix(msg.Title,
		"shop: 5 of 15 workloads failing since 10:02") ||
		!strings.HasSuffix(msg.Title, ".") {
		t.Fatalf("title = %q, want a sentence", msg.Title)
	}
	if !msg.Opens || !strings.HasPrefix(msg.Key, "rollup/") ||
		!strings.HasSuffix(msg.Key, "/shop") {
		t.Fatalf("key = %q opens = %v", msg.Key, msg.Opens)
	}
	// Five workloads in the same state are one line that names each.
	if len(msg.Lines) != 1 ||
		!strings.HasPrefix(msg.Lines[0], "a, b, c, d and e in shop") {
		t.Fatalf("want one line naming all five, got %q", msg.Lines)
	}
}

func TestNamespaceOutageOmitsAnUnknownTotalAndNamesNoCause(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	ds := outageCases(5, now)

	msg := Writer{}.NamespaceOutage(
		NamespaceOutage{Namespace: "shop"}, ds, now)

	if !strings.HasPrefix(msg.Title, "shop: 5 workloads failing") {
		t.Fatalf("title = %q", msg.Title)
	}
	for _, word := range []string{"because", "caused", "root cause"} {
		if strings.Contains(msg.Note, word) {
			t.Fatalf("an outage must not invent a cause: %q", msg.Note)
		}
	}
}

func TestNamespaceOutageStatesWhatIsShared(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	change := inventory.Change{
		Entity: inventory.EntityID{Kind: kube.KindConfigMap,
			Namespace: "shop", Name: "app-config"},
		At: time.Date(2026, 9, 29, 10, 1, 0, 0, time.UTC), Actor: "bob",
	}
	o := NamespaceOutage{Namespace: "shop",
		Shared: OutageShared{Node: "n1", Change: &change}}

	msg := Writer{}.NamespaceOutage(o, outageCases(5, now), now)

	if !strings.Contains(msg.Note, "They all ran on node n1.") ||
		!strings.Contains(msg.Note, "Shortly before, bob changed") ||
		!strings.Contains(msg.Note, "at 10:01 UTC") {
		t.Fatalf("note = %q", msg.Note)
	}
}

// The shared change is Pro detail: Community keeps the shared node,
// names no change and adds no line about it.
func TestNamespaceOutageCommunityNamesNoSharedChange(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	change := inventory.Change{
		Entity: inventory.EntityID{Kind: kube.KindConfigMap,
			Namespace: "shop", Name: "app-config"},
		At: time.Date(2026, 9, 29, 10, 1, 0, 0, time.UTC), Actor: "bob",
	}
	o := NamespaceOutage{Namespace: "shop",
		Shared: OutageShared{Node: "n1", Change: &change}}
	w := Writer{}.ForEdition(feature.CommunityEntitlements(), false)

	msg := w.NamespaceOutage(o, outageCases(5, now), now)

	if !strings.Contains(msg.Note, "They all ran on node n1.") {
		t.Fatalf("note = %q", msg.Note)
	}
	for _, word := range []string{"Shortly before", "app-config", "bob",
		"Pro"} {
		if strings.Contains(msg.Note, word) {
			t.Errorf("community note says %q: %q", word, msg.Note)
		}
	}
	for _, b := range msg.Doc {
		for _, sp := range b.Spans {
			if strings.Contains(sp.Text, "app-config") {
				t.Errorf("community doc names the change: %q", sp.Text)
			}
		}
	}
}

func TestNamespaceOutageIsAsLoudAsItsLoudestMember(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)
	ds := outageCases(5, now)
	ds[3].Incident.Tier = incident.Page

	msg := Writer{}.NamespaceOutage(
		NamespaceOutage{Namespace: "shop"}, ds, now)

	if msg.Status != notification.StatusCritical {
		t.Fatalf("status = %v", msg.Status)
	}
}

func TestNamespaceOutageNamesTheCluster(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 5, 0, 0, time.UTC)

	msg := Writer{Cluster: "prod-eu-1"}.NamespaceOutage(
		NamespaceOutage{Namespace: "shop"}, outageCases(5, now), now)

	if !strings.HasPrefix(msg.Title, "shop (prod-eu-1): 5 workloads") {
		t.Fatalf("title = %q", msg.Title)
	}
}
