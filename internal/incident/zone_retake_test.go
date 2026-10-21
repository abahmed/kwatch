package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

func zoneA() inventory.EntityID {
	return inventory.CoreID(kube.KindZone, "", "zone-a")
}

// zoneRig announces the page of node a1, in zone-a with a2.
func zoneRig(t *testing.T) (*rig, string) {
	t.Helper()
	r := newRig(t, Config{})
	for _, name := range []string{"a1", "a2"} {
		r.relate(inventory.CoreID(kube.KindNode, "", name),
			inventory.PartOf, zoneA())
	}
	r.raise(at(0), lostNode("a1"))
	ds := r.tick(at(DefaultPageSettle))
	wantAction(t, ds, Announce, ReasonSettled)
	require.Equal(t, Page, ds[0].Incident.Tier)
	return r, ds[0].Incident.ID
}

// The second node of a zone goes down minutes after the first paged, and
// the engine now says the zone fails as a whole. It is one outage: the
// incident that paged takes the zone as its root and says its cause was
// revised. No second page, no "moved" closing the first alert while the
// outage goes on.
func TestZoneFailingAfterANodePagedIsTheSameIncident(t *testing.T) {
	r, id := zoneRig(t)
	a2 := lostNode("a2")
	r.rule.causes[a2.Entity] = explain.Cause{Root: zoneA(),
		Row: explain.RowZoneFailing, Summary: "zone-a is failing",
		Confidence: 0.9}
	now := at(4 * time.Minute)
	r.raise(now, a2)

	var told []Decision
	for end := now.Add(2 * time.Minute); now.Before(end); now =
		now.Add(10 * time.Second) {
		told = append(told, r.tick(now)...)
	}

	require.Len(t, told, 1, "one update, no new announcement")
	assert.Equal(t, Update, told[0].Action)
	assert.Equal(t, ReasonCauseRevised, told[0].Reason)
	assert.Equal(t, id, told[0].Incident.ID)
	assert.Equal(t, zoneA(), told[0].Incident.Root)
	assert.Equal(t, Page, told[0].Incident.Tier)
	assert.Len(t, r.m.Export(), 1, "one incident speaks for the outage")
}

// A zone that no node incident announced speaks for itself, as before.
func TestZoneFailingWithoutAnAnnouncedNodeKeepsItsOwnIncident(t *testing.T) {
	r := newRig(t, Config{})
	for _, name := range []string{"a1", "a2"} {
		r.relate(inventory.CoreID(kube.KindNode, "", name),
			inventory.PartOf, zoneA())
	}
	a2 := lostNode("a2")
	r.rule.causes[a2.Entity] = explain.Cause{Root: zoneA(),
		Row: explain.RowZoneFailing, Summary: "zone-a is failing",
		Confidence: 0.9}
	r.raise(at(0), a2)

	ds := r.tick(at(DefaultPageSettle))

	wantAction(t, ds, Announce, ReasonSettled)
	assert.Equal(t, zoneA(), ds[0].Incident.Root)
}
