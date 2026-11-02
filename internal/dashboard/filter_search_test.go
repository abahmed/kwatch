package dashboard

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/rootcause"
)

// searchFixture is filterFixture with a node incident and a cause, for
// the search shortcuts the list sends: kind, exclude and several q.
func searchFixture() *Server {
	api := testIncident("a", "pay", 0)
	api.Headline = "payments-api keeps crashing"
	worker := testIncident("w", "pay", -5*time.Minute)
	worker.Headline = "payments worker out of memory"
	worker.Cause = &rootcause.CauseRecord{Summary: "the memory limit is low"}
	node := testIncident("n", "", -10*time.Minute)
	node.Root.Kind = "Node"
	node.Root.Name = "node-1"
	node.Headline = "node-1 is not ready"
	shop := testIncident("s", "shop", -15*time.Minute)
	shop.Headline = "shop keeps crashing"
	return newTestServer(api, worker, node, shop)
}

func TestFilterSearchTermsMustAllMatchAndExcludesDrop(t *testing.T) {
	s := searchFixture()
	cases := map[string]string{
		"/api/incidents?q=payments":                        "a,w",
		"/api/incidents?q=payments&q=memory":               "w",
		"/api/incidents?q=crashing&q=shop":                 "s",
		"/api/incidents?q=out+of+memory":                   "w",
		"/api/incidents?q=payments&exclude=worker":         "a",
		"/api/incidents?exclude=crashing":                  "w,n",
		"/api/incidents?exclude=crashing&exclude=memory":   "n",
		"/api/incidents?q=&exclude=":                       "a,w,n,s",
		"/api/incidents?q=PAYMENTS&exclude=Keeps+Crashing": "w",
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

func TestFilterKindAndNamespacesNarrowTheList(t *testing.T) {
	s := searchFixture()
	cases := map[string]string{
		"/api/incidents?kind=node":                    "n",
		"/api/incidents?kind=Deployment":              "a,w,s",
		"/api/incidents?kind=node&kind=deployment":    "a,w,n,s",
		"/api/incidents?kind=statefulset":             "",
		"/api/incidents?namespace=pay&namespace=shop": "a,w,s",
		"/api/incidents?namespace=pay&namespace=shop&kind=Deployment": "" +
			"a,w,s",
		"/api/incidents?namespace=shop&q=crashing": "s",
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

func TestFilterSearchLengthCountsEveryTerm(t *testing.T) {
	s := searchFixture()
	half := strings.Repeat("x", maxQueryLength/2+1)
	for _, path := range []string{
		"/api/incidents?q=" + half + "&q=" + half,
		"/api/incidents?q=" + half + "&exclude=" + half,
	} {
		if w := get(t, s, path); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path[:40], w.Code)
		}
	}
}

// Each search word is looked for in every incident, so their number is
// capped as well as their length.
func TestFilterSearchCapsTheNumberOfWords(t *testing.T) {
	s := searchFixture()
	ten := "/api/incidents?q=a&q=b&q=c&q=d&q=e" +
		"&exclude=f&exclude=g&exclude=h&exclude=i&exclude=j"
	if w := get(t, s, ten); w.Code != http.StatusOK {
		t.Errorf("%d words: %d, want 200", maxSearchTerms, w.Code)
	}
	w := get(t, s, ten+"&q=k")
	if w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "too many search words") {
		t.Errorf("11 words: %d %s", w.Code, w.Body.String())
	}
}

// The text searched is lower-cased once per incident, not once per
// word: the haystack already holds it in lower case.
func TestFilterSearchableIsLowerCasedOnce(t *testing.T) {
	p := testIncident("A1", "Shop", 0)
	p.Headline = "Payments-API Keeps Crashing"
	h := searchable(p, toListItemWith(p, redactNone), redactNone)
	for _, s := range h {
		if s != strings.ToLower(s) {
			t.Errorf("haystack holds %q, not lower case", s)
		}
	}
	if !h.contains("payments-api keeps") || !h.contains("a1") {
		t.Errorf("haystack %q misses its own text", h)
	}
}
