package dashboard

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// filterFixture is a small cluster: urgent, notify, low and quiet active
// incidents and two resolved ones.
func filterFixture() *Server {
	urgent := testIncident("u", "pay", 0)
	urgent.Tier = incident.Page
	urgent.Headline = "payments-api keeps crashing"
	notify := testIncident("n", "shop", 2*time.Minute)
	notify.State = incident.Flapping
	notify.Cause = &rootcause.CauseRecord{Summary: "the database is down"}
	low := testIncident("l", "shop", 3*time.Minute)
	low.Tier = incident.Digest
	quiet := testIncident("q", "web", 4*time.Minute)
	quiet.Tier = incident.Silent
	early := testIncident("e", "pay", -2*time.Hour)
	early.State = incident.Resolved
	early.Resolved = testEpoch.Add(-time.Minute)
	late := testIncident("r", "web", -time.Hour)
	late.State = incident.Resolved
	late.Resolved = testEpoch.Add(-30 * time.Hour)
	return newTestServer(urgent, notify, low, quiet, early, late)
}

func TestFilterNarrowsTheListOnTheServer(t *testing.T) {
	s := filterFixture()
	cases := map[string]string{
		"/api/incidents":                                 "u,n,l",
		"/api/incidents?severity=silent":                 "q",
		"/api/incidents?severity=page&severity=notify":   "u,n",
		"/api/incidents?severity=page&severity=digest":   "u,l",
		"/api/incidents?state=flapping":                  "n",
		"/api/incidents?state=flapping&state=open":       "u,n,l",
		"/api/incidents?q=PAYMENTS":                      "u",
		"/api/incidents?q=database":                      "n",
		"/api/incidents?q=shop+/":                        "n,l",
		"/api/incidents?namespace=shop":                  "n,l",
		"/api/incidents?status=resolved":                 "e,r",
		"/api/incidents?status=resolved&since=24h":       "e",
		"/api/incidents?status=resolved&since=31h":       "e,r",
		"/api/incidents?status=resolved&state=open":      "",
		"/api/incidents?status=all&state=resolved&q=pay": "e",
	}
	for path, want := range cases {
		w := get(t, s, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if got := listIDs(decodeList(t, w.Body.String())); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestFilterOrdersUrgentFirstAndLatestResolvedFirst(t *testing.T) {
	s := filterFixture()
	active := decodeList(t, get(t, s, "/api/incidents").Body.String())
	if active.Incidents[0].Severity != "page" {
		t.Errorf("first active = %+v, want the urgent one", active.Incidents[0])
	}
	resolved := decodeList(t,
		get(t, s, "/api/incidents?status=resolved").Body.String())
	if listIDs(resolved) != "e,r" {
		t.Errorf("resolved order = %s, want e (resolved later) first",
			listIDs(resolved))
	}
}

func TestFilterRejectsUnknownValues(t *testing.T) {
	s := filterFixture()
	for _, path := range []string{
		"/api/incidents?severity=critical",
		"/api/incidents?severity=page,notify",
		"/api/incidents?state=active",
		"/api/incidents?status=open",
		"/api/incidents?since=yesterday",
		"/api/incidents?since=2026-01-01T00:00:00Z",
		"/api/incidents?q=" + strings.Repeat("x", maxQueryLength+1),
	} {
		if w := get(t, s, path); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, w.Code)
		}
	}
}

func TestFilterNarrowsByWhenTheIssueStarted(t *testing.T) {
	recent := testIncident("new", "pay", -10*time.Minute)
	old := testIncident("old", "pay", -30*time.Hour)
	gone := testIncident("gone", "pay", -40*time.Hour)
	gone.State = incident.Resolved
	gone.Resolved = testEpoch.Add(-time.Hour)
	s := newTestServer(recent, old, gone)
	cases := map[string]string{
		"/api/incidents?startedAfter=24h":                  "new",
		"/api/incidents?startedBefore=24h":                 "old",
		"/api/incidents?startedAfter=1h":                   "new",
		"/api/incidents?status=resolved&startedBefore=24h": "gone",
		"/api/incidents?startedAfter=48h&startedBefore=24h": "" +
			"old",
	}
	for path, want := range cases {
		w := get(t, s, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if got := listIDs(decodeList(t, w.Body.String())); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
	for _, path := range []string{
		"/api/incidents?startedAfter=today",
		"/api/incidents?startedBefore=-1h",
		"/api/incidents?startedAfter=2026-01-01T00:00:00Z",
	} {
		if w := get(t, s, path); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, w.Code)
		}
	}
}
