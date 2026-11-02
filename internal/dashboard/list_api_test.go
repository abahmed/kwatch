package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// manyIncidents is n active incidents over five namespaces, opened a
// second apart, with every tenth urgent and every seventh flapping.
func manyIncidents(n int) []incident.Incident {
	items := make([]incident.Incident, 0, n)
	for i := range n {
		p := testIncident("i"+strconv.Itoa(i), "ns"+strconv.Itoa(i%5),
			time.Duration(i)*time.Second)
		if i%10 == 0 {
			p.Tier = incident.Page
		}
		if i%7 == 0 {
			p.State = incident.Flapping
		}
		items = append(items, p)
	}
	return items
}

func TestListStatusPicksActiveResolvedOrAll(t *testing.T) {
	s := filterFixture()
	cases := map[string]string{
		"/api/incidents?status=active":                 "u,n,l",
		"/api/incidents?status=resolved":               "e,r",
		"/api/incidents?status=all":                    "l,n,u,e,r",
		"/api/incidents?status=all&since=24h":          "l,n,u,e",
		"/api/incidents?status=all&state=flapping":     "n",
		"/api/incidents":                               "u,n,l",
		"/api/incidents?status=active&acked=false":     "u,n,l",
		"/api/incidents?status=active&acked=true":      "",
		"/api/incidents?status=resolved&severity=page": "",
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

func TestListSortAcceptsOnlyKnownFieldsBothWays(t *testing.T) {
	s := filterFixture()
	cases := map[string]string{
		"/api/incidents?status=active&sort=severity":           "u,n,l",
		"/api/incidents?status=active&sort=severity&dir=asc":   "l,n,u",
		"/api/incidents?status=active&sort=started&dir=asc":    "u,n,l",
		"/api/incidents?status=active&sort=status":             "l,u,n",
		"/api/incidents?status=resolved&sort=duration":         "e,r",
		"/api/incidents?status=active&sort=namespace&dir=desc": "l,n,u",
		"/api/incidents?status=active&sort=last":               "l,n,u",
	}
	for path, want := range cases {
		if got := listIDs(decodeList(t,
			get(t, s, path).Body.String())); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestListRejectsInvalidParamsWithAPlainMessage(t *testing.T) {
	s := filterFixture()
	cases := map[string]string{
		"/api/incidents?sort=password":      "invalid sort",
		"/api/incidents?sort=recent":        "invalid sort",
		"/api/incidents?sort=title&dir=up":  "invalid dir",
		"/api/incidents?status=gone":        "invalid status",
		"/api/incidents?page=-1":            "invalid page",
		"/api/incidents?page=x":             "invalid page",
		"/api/incidents?pageSize=1000":      "invalid pageSize",
		"/api/incidents?acked=maybe":        "invalid acked",
		"/api/incidents?since=1d":           "invalid since",
		"/api/incidents?startedBefore=soon": "invalid startedBefore",
	}
	for path, want := range cases {
		w := get(t, s, path)
		if w.Code != http.StatusBadRequest ||
			!strings.Contains(w.Body.String(), `"error":"`+want+`"`) {
			t.Errorf("%s: %d %s, want 400 %q", path, w.Code,
				w.Body.String(), want)
		}
	}
}

func TestListPagesAreBoundedAndStable(t *testing.T) {
	s := newTestServer(manyIncidents(230)...)
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		path := "/api/incidents?sort=severity&pageSize=100&page=" +
			strconv.Itoa(page)
		l := decodeList(t, get(t, s, path).Body.String())
		again := decodeList(t, get(t, s, path).Body.String())
		if listIDs(l) != listIDs(again) || l.TotalPages != 3 ||
			l.Page != page {
			t.Fatalf("page %d: differs or %d pages", page, l.TotalPages)
		}
		for _, item := range l.Incidents {
			if seen[item.ID] {
				t.Fatalf("%s on two pages", item.ID)
			}
			seen[item.ID] = true
		}
	}
	past := decodeList(t, get(t, s, "/api/incidents?page=50").
		Body.String())
	if len(seen) != 230 || len(past.Incidents) != 0 ||
		past.Total != 230 || past.Page != 50 {
		t.Errorf("pages covered %d, past the end %d of %d", len(seen),
			len(past.Incidents), past.Total)
	}
}

func TestListSortTiesBreakByID(t *testing.T) {
	items := make([]incident.Incident, 0, 30)
	for i := range 30 {
		items = append(items, testIncident("i"+strconv.Itoa(29-i),
			"ns", 0))
	}
	s := newTestServer(items...)
	asc := listIDs(decodeList(t, get(t, s,
		"/api/incidents?sort=namespace&pageSize=25").Body.String()))
	desc := listIDs(decodeList(t, get(t, s,
		"/api/incidents?sort=namespace&dir=desc&pageSize=25").
		Body.String()))
	if !strings.HasPrefix(asc, "i0,i1,i10,i11,") || asc != desc {
		t.Errorf("ties: asc %s, desc %s, want ID order both",
			asc[:20], desc[:20])
	}
}

func TestListFacetsCountTheWholeFilteredSet(t *testing.T) {
	s := newTestServer(manyIncidents(300)...)
	l := decodeList(t, get(t, s,
		"/api/incidents?status=active&severity=page&pageSize=25").
		Body.String())
	if l.Total != 30 || len(l.Incidents) != 25 || l.TotalPages != 2 {
		t.Fatalf("total %d, %d items", l.Total, len(l.Incidents))
	}
	// The severity facet ignores the severity filter; the others don't.
	if l.Facets.Severity["page"] != 30 || l.Facets.Severity["notify"] != 270 {
		t.Errorf("severity facet = %v", l.Facets.Severity)
	}
	if l.Facets.State["flapping"]+l.Facets.State["open"] != 30 {
		t.Errorf("state facet = %v", l.Facets.State)
	}
	if len(l.Facets.Namespace) != 1 || l.Facets.Namespace["ns0"] != 30 {
		t.Errorf("namespace facet = %v", l.Facets.Namespace)
	}
}

func TestListNamespaceFacetKeepsTheBusiest(t *testing.T) {
	items := make([]incident.Incident, 0, maxNamespaceFacets+10)
	for i := range maxNamespaceFacets + 10 {
		items = append(items, testIncident("i"+strconv.Itoa(i),
			"ns"+strconv.Itoa(i), 0))
	}
	items = append(items, testIncident("x", "ns3", time.Second))
	l := decodeList(t, get(t, newTestServer(items...),
		"/api/incidents").Body.String())
	ns := l.Facets.Namespace
	if len(ns) != maxNamespaceFacets || ns["ns3"] != 2 {
		t.Errorf("namespace facet = %v", ns)
	}
}

func TestListETagAnswers304UntilTheListChanges(t *testing.T) {
	reader := &fakeIncidents{items: manyIncidents(20)}
	s := newTestServer()
	s.SetIncidents(reader)
	const path = "/api/incidents?status=active"
	first := get(t, s, path)
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag")
	}
	if w := getIfNoneMatch(t, s, path, tag); w.Code !=
		http.StatusNotModified || w.Body.Len() != 0 {
		t.Errorf("unchanged: %d with %d bytes", w.Code, w.Body.Len())
	}
	reader.mu.Lock()
	reader.items[3].RepeatCount++
	reader.mu.Unlock()
	w := getIfNoneMatch(t, s, path, tag)
	if w.Code != http.StatusOK || w.Header().Get("ETag") == tag {
		t.Errorf("changed: %d, same tag", w.Code)
	}
}

func TestListFullPageStaysWithin64KiBWithManyIncidents(t *testing.T) {
	items := manyIncidents(5000)
	for i := range items {
		items[i].Headline = strings.Repeat("a <very> long title ", 40)
		items[i].Root.Namespace = strings.Repeat("n", 60) +
			strconv.Itoa(i%60)
		items[i].Root.Name = strings.Repeat("w", 250)
		items[i].Root.Kind = inventory.Kind("Kind" + strconv.Itoa(i%40))
		items[i].Cause = &rootcause.CauseRecord{
			Summary: strings.Repeat("cause & ", 80)}
	}
	s := newTestServer(items...)
	for _, path := range []string{"/api/incidents?pageSize=100",
		"/api/incidents?pageSize=100&page=50&sort=title"} {
		w := get(t, s, path)
		if w.Code != http.StatusOK || w.Body.Len() > maxResponseBytes {
			t.Fatalf("%s: %d, %d bytes", path, w.Code, w.Body.Len())
		}
		l := decodeList(t, w.Body.String())
		if l.Total != 5000 || len(l.Incidents) != 100 ||
			l.TotalPages != 50 {
			t.Errorf("%s: total %d, %d items", path, l.Total,
				len(l.Incidents))
		}
	}
}

func getIfNoneMatch(t *testing.T, s *Server, path, tag string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("If-None-Match", tag)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}
