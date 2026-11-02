package dashboard

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// similarFixture: a, b and r (resolved) cannot pull their image, q too
// but quietly, c crash-loops, i cannot pull its image for another
// cause and n has no mode at all.
func similarFixture() *Server {
	pull := func(id string, opened time.Duration) incident.Incident {
		p := testIncident(id, "team-"+id, opened)
		p.Mode = detection.Mode("ImagePull")
		return p
	}
	a, b := pull("a", 0), pull("b", time.Minute)
	r := pull("r", -time.Hour)
	r.State, r.Resolved = incident.Resolved, testEpoch.Add(-time.Minute)
	q := pull("q", 2*time.Minute)
	q.Tier = incident.Silent
	i := pull("i", 3*time.Minute)
	i.Cause = &rootcause.CauseRecord{Rule: "registry-down"}
	c := testIncident("c", "web", 0)
	c.Mode = detection.ModeCrashLoop
	n := testIncident("n", "web", 0)
	return newTestServer(a, b, r, q, i, c, n)
}

func TestSimilarKeyIsTheModeAndTheCauseRule(t *testing.T) {
	p := incident.Incident{Mode: "ImagePull"}
	withRule := p
	withRule.Cause = &rootcause.CauseRecord{Rule: "registry-down"}
	ruleOnly := incident.Incident{
		Cause: &rootcause.CauseRecord{Rule: "registry-down"}}
	keys := []string{similarKey(p), similarKey(withRule),
		similarKey(ruleOnly)}
	for i, k := range keys {
		for j := range i {
			if k == keys[j] {
				t.Errorf("keys %d and %d are both %q", j, i, k)
			}
		}
	}
	if k := similarKey(incident.Incident{}); k != "" {
		t.Errorf("no mode and no cause: key %q, want none", k)
	}
	summaryOnly := incident.Incident{
		Cause: &rootcause.CauseRecord{Summary: "x"}}
	if k := similarKey(summaryOnly); k != "" {
		t.Errorf("a cause without a rule: key %q, want none", k)
	}
}

func TestListLikeKeepsOnlyIncidentsWithTheSameError(t *testing.T) {
	s := similarFixture()
	cases := map[string]string{
		"/api/incidents?status=all&like=a":                 "b,r",
		"/api/incidents?status=active&like=a":              "b",
		"/api/incidents?status=all&like=a&severity=silent": "q",
		"/api/incidents?status=all&like=i":                 "",
		"/api/incidents?status=all&like=n":                 "",
		"/api/incidents?status=all&like=gone":              "",
		"/api/incidents?status=all&like=":                  "a,b,i,c,n,r",
	}
	for path, want := range cases {
		w := get(t, s, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		got := listIDs(decodeList(t, w.Body.String()))
		if !sameIDs(got, want) {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestListLikeRejectsTwoIDsOrALongOne(t *testing.T) {
	s := similarFixture()
	for _, path := range []string{"/api/incidents?like=a&like=b",
		"/api/incidents?like=" + strings.Repeat("x", maxLikeLength+1)} {
		w := get(t, s, path)
		if w.Code != http.StatusBadRequest ||
			!strings.Contains(w.Body.String(), `"param":"like"`) {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestDetailCountsWhatTheLikeListShows(t *testing.T) {
	s := similarFixture()
	for id, want := range map[string]int{"a": 2, "b": 2, "r": 2, "q": 3,
		"i": 0, "c": 0, "n": 0} {
		d := detailOf(t, s, id)
		if d.Similar != want {
			t.Errorf("%s: similar = %d, want %d", id, d.Similar, want)
		}
		if d.Similar == 0 && strings.Contains(
			get(t, s, "/api/incidents/"+id).Body.String(), `"similar"`) {
			t.Errorf("%s: similar 0 is in the response", id)
		}
	}
	listed := decodeList(t, get(t, s,
		"/api/incidents?status=all&like=b").Body.String())
	if listed.Total != detailOf(t, s, "b").Similar {
		t.Errorf("like list %d, detail %d", listed.Total,
			detailOf(t, s, "b").Similar)
	}
}

// sameIDs compares comma-separated IDs in any order.
func sameIDs(got, want string) bool {
	g, w := strings.Split(got, ","), strings.Split(want, ",")
	if len(g) != len(w) {
		return false
	}
	seen := map[string]int{}
	for _, id := range g {
		seen[id]++
	}
	for _, id := range w {
		if seen[id]--; seen[id] < 0 {
			return false
		}
	}
	return true
}
