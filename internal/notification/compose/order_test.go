package compose

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func orderCase(
	name string, tier incident.Tier, impact ...inventory.EntityID,
) incident.Decision {
	return incident.Decision{Incident: incident.Incident{
		Root: inventory.EntityID{
			Kind: kube.KindDeployment, Namespace: "shop", Name: name},
		Tier: tier, Impact: impact,
	}}
}

// Lists start with what matters most: tier, then lost traffic, then
// the larger impact, then the name.
func TestSortByImpactOrdersTierTrafficSizeName(t *testing.T) {
	svc := inventory.EntityID{
		Kind: kube.KindService, Namespace: "shop", Name: "cart"}
	pod := func(n string) inventory.EntityID {
		return inventory.EntityID{
			Kind: kube.KindDeployment, Namespace: "shop", Name: n}
	}
	list := []incident.Decision{
		orderCase("zeta", incident.Notify),
		orderCase("big", incident.Notify, pod("a"), pod("b")),
		orderCase("alpha", incident.Notify),
		orderCase("web", incident.Notify, svc),
		orderCase("pager", incident.Page),
	}

	sortByImpact(list)

	var got []string
	for _, d := range list {
		got = append(got, d.Incident.Root.Name)
	}
	want := []string{"pager", "web", "big", "alpha", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// The digest lists its titles in that order without reordering the
// caller's slice.
func TestDigestTitlesLeaveCallerOrderAlone(t *testing.T) {
	list := []incident.Decision{
		orderCase("a", incident.Digest), orderCase("b", incident.Notify),
	}
	titles := digestTitles(list, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), "")
	if len(titles) != 2 || list[0].Incident.Root.Name != "a" {
		t.Fatalf("caller slice reordered or titles lost: %+v", titles)
	}
}
