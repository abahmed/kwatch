package dashboard

import (
	"net/http"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// severityFixture: two issues of each level the list shows by default,
// the second of each newer.
func severityFixture() *Server {
	var items []incident.Incident
	tiers := map[string]incident.Tier{"l": incident.Digest,
		"n": incident.Notify, "u": incident.Page}
	for name, tier := range tiers {
		for i, at := range []time.Duration{-2 * time.Hour, -time.Hour} {
			p := testIncident(name+string(rune('1'+i)), "a", at)
			p.Tier = tier
			items = append(items, p)
		}
	}
	return newTestServer(items...)
}

func TestListOrderDefaultIsMostSevereThenNewest(t *testing.T) {
	s := severityFixture()
	w := get(t, s, "/api/incidents?status=active")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	got := listIDs(decodeList(t, w.Body.String()))
	if want := "u2,u1,n2,n1,l2,l1"; got != want {
		t.Errorf("default order = %s, want %s", got, want)
	}
}

func TestListOrderSeverityKeepsNewestFirstWithinALevel(t *testing.T) {
	s := severityFixture()
	got := listIDs(decodeList(t, get(t, s,
		"/api/incidents?status=active&sort=severity&dir=asc").Body.String()))
	if want := "l2,l1,n2,n1,u2,u1"; got != want {
		t.Errorf("severity asc = %s, want %s", got, want)
	}
}
