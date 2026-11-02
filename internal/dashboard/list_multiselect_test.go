package dashboard

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// multiFixture is four active incidents of two kinds, one acknowledged,
// and a resolved one: a = Deployment in pay (urgent, acknowledged),
// b = Node cluster-wide (flapping), c = StatefulSet in shop, d =
// Deployment in shop (low), z = resolved Deployment in pay.
func multiFixture() *Server {
	a := testIncident("a", "pay", 0)
	a.Tier = incident.Page
	a.Ack = &incident.Ack{On: a.Root, Note: "on it"}
	b := testIncident("b", "", time.Minute)
	b.Root = inventory.EntityID{Kind: "Node", Name: "node-1"}
	b.State = incident.Flapping
	c := testIncident("c", "shop", 2*time.Minute)
	c.Root.Kind = "StatefulSet"
	d := testIncident("d", "shop", 3*time.Minute)
	d.Tier = incident.Digest
	z := testIncident("z", "pay", -time.Hour)
	z.State = incident.Resolved
	z.Resolved = testEpoch.Add(-time.Minute)
	return newTestServer(a, b, c, d, z)
}

// sortedIDs is the IDs of a list, A to Z, for filters whose order
// doesn't matter here.
func sortedIDs(l ListResponse) string {
	ids := strings.Split(listIDs(l), ",")
	slices.Sort(ids)
	return strings.Join(ids, ",")
}

func TestListRepeatableFiltersAreOredWithinAndAndedAcross(t *testing.T) {
	s := multiFixture()
	cases := map[string]string{
		"/api/incidents?status=active&acked=true&acked=false": "a,b,c,d",
		"/api/incidents?status=active&acked=false":            "b,c,d",
		"/api/incidents?status=active&acked=true":             "a",
		"/api/incidents?status=active&kind=node&kind=Deployment": "" +
			"a,b,d",
		"/api/incidents?status=active&namespace=pay&namespace=shop": "" +
			"a,c,d",
		"/api/incidents?status=active&namespace=shop&kind=deployment": "d",
		"/api/incidents?status=active&severity=page&severity=digest" +
			"&acked=false": "d",
		"/api/incidents?status=all&state=flapping&state=resolved": "b,z",
		"/api/incidents?status=all&state=open&state=resolved":     "a,c,d,z",
		"/api/incidents?state=open&state=resolved":                "a,c,d",
		"/api/incidents?status=active&state=open&state=flapping":  "a,b,c,d",
	}
	for path, want := range cases {
		w := get(t, s, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if got := sortedIDs(decodeList(t, w.Body.String())); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestListRepeatableFiltersRefuseUnknownValues(t *testing.T) {
	s := multiFixture()
	cases := map[string]string{
		"/api/incidents?acked=true&acked=maybe": "invalid acked",
		"/api/incidents?acked=true,false":       "invalid acked",
		"/api/incidents?kind=node&status=done":  "invalid status",
		"/api/incidents?state=open&state=bogus": "invalid state",
	}
	for path, want := range cases {
		w := get(t, s, path)
		if w.Code != http.StatusBadRequest ||
			!strings.Contains(w.Body.String(), want) {
			t.Errorf("%s: %d %s, want %q", path, w.Code, w.Body.String(),
				want)
		}
	}
}

func TestListFacetsCountKindsAndAckedWithoutTheirOwnFilter(t *testing.T) {
	s := multiFixture()
	l := decodeList(t, get(t, s,
		"/api/incidents?status=active&kind=deployment&acked=false").
		Body.String())
	if got := listIDs(l); got != "d" {
		t.Fatalf("list = %s, want d", got)
	}
	want := map[string]int{"Deployment": 1, "Node": 1,
		"StatefulSet": 1}
	if !maps.Equal(l.Facets.Kind, want) {
		t.Errorf("kinds = %v, want %v (acked=false, any kind)",
			l.Facets.Kind, want)
	}
	if l.Facets.Acked["true"] != 1 || l.Facets.Acked["false"] != 1 {
		t.Errorf("acked = %v, want 1 and 1 (deployments, any ack)",
			l.Facets.Acked)
	}
	if l.Facets.Severity["digest"] != 1 || l.Facets.Severity["page"] != 0 {
		t.Errorf("severity = %v, want only the low one",
			l.Facets.Severity)
	}
}

func TestListKindFacetKeepsTheBusiestKinds(t *testing.T) {
	items := make([]incident.Incident, 0, maxKindFacets+2)
	for i := range maxKindFacets + 2 {
		p := testIncident(string(rune('a'+i)), "ns", 0)
		p.Root.Kind = inventory.Kind("Kind" + string(rune('A'+i)))
		items = append(items, p)
	}
	items = append(items, testIncident("zz", "ns", 0))
	items[len(items)-1].Root.Kind = "KindA"
	l := decodeList(t, get(t, newTestServer(items...),
		"/api/incidents?status=active").Body.String())
	kinds := l.Facets.Kind
	if len(kinds) != maxKindFacets || kinds["KindA"] != 2 {
		t.Fatalf("kinds = %v, want %d with KindA at 2", kinds,
			maxKindFacets)
	}
	// Ties keep the first A to Z: KindB to KindT, not KindU or KindV.
	if _, ok := kinds["KindV"]; ok || kinds["KindT"] != 1 {
		t.Errorf("kinds = %v, want the first A to Z on a tie", kinds)
	}
}
