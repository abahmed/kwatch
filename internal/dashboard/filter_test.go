package dashboard

import (
	"encoding/json"
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
		"/api/incidents?state=active":                               "u,l,n",
		"/api/incidents?state=active&severity=silent":               "q",
		"/api/incidents?state=active&severity=page,notify":          "u,n",
		"/api/incidents?state=active&severity=page&severity=digest": "u,l",
		"/api/incidents?state=active&state=flapping":                "n",
		"/api/incidents?state=flapping&state=open":                  "u,l,n",
		"/api/incidents?state=active&q=PAYMENTS":                    "u",
		"/api/incidents?state=active&q=database":                    "n",
		"/api/incidents?state=active&q=shop+/":                      "l,n",
		"/api/incidents?state=active&namespace=shop":                "l,n",
		"/api/incidents?state=resolved":                             "e,r",
		"/api/incidents?state=resolved&since=24h":                   "e",
		"/api/incidents?state=resolved&since=" +
			testEpoch.Add(-31*time.Hour).Format(time.RFC3339): "e,r",
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
	active := decodeList(t, get(t, s, "/api/incidents?state=active").Body.String())
	if active.Incidents[0].Severity != "page" {
		t.Errorf("first active = %+v, want the urgent one", active.Incidents[0])
	}
	resolved := decodeList(t,
		get(t, s, "/api/incidents?state=resolved").Body.String())
	if listIDs(resolved) != "e,r" {
		t.Errorf("resolved order = %s, want e (resolved later) first",
			listIDs(resolved))
	}
}

func TestFilterCountsIgnoreTheOtherFiltersAndTheLimit(t *testing.T) {
	s := filterFixture()
	w := get(t, s, "/api/incidents?state=active&severity=page&limit=1")
	l := decodeList(t, w.Body.String())
	if l.Total != 1 || l.Counts["page"] != 1 || l.Counts["notify"] != 1 ||
		l.Counts["digest"] != 1 || l.Counts["silent"] != 0 {
		t.Errorf("total %d counts %v", l.Total, l.Counts)
	}
	empty := decodeList(t, get(t, newTestServer(),
		"/api/incidents?state=active").Body.String())
	if empty.Counts == nil || empty.Counts["page"] != 0 {
		t.Errorf("empty counts = %v, want zeros", empty.Counts)
	}
}

func TestFilterRejectsUnknownValues(t *testing.T) {
	s := filterFixture()
	for _, path := range []string{
		"/api/incidents?severity=critical",
		"/api/incidents?state=resolved&state=open",
		"/api/incidents?state=active&state=resolved",
		"/api/incidents?since=yesterday",
		"/api/incidents?q=" + strings.Repeat("x", maxQueryLength+1),
		"/api/namespaces?state=bogus",
	} {
		if w := get(t, s, path); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, w.Code)
		}
	}
}

func TestFilterNamespacesListsTheNamespacesOfAList(t *testing.T) {
	s := filterFixture()
	cases := map[string]string{
		"/api/namespaces?state=active":   "pay,shop",
		"/api/namespaces?state=resolved": "pay,web",
		"/api/namespaces":                "pay,shop,web",
	}
	for path, want := range cases {
		var out NamespacesResponse
		w := get(t, s, path)
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got := strings.Join(out.Namespaces, ","); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
	s.SetIncidents(nil)
	if w := get(t, s, "/api/namespaces"); w.Code != 503 {
		t.Errorf("without a pipeline: %d, want 503", w.Code)
	}
}
