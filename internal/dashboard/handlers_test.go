package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

func decodeList(t *testing.T, body string) ListResponse {
	t.Helper()
	var out ListResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("list body %q: %v", body, err)
	}
	return out
}

func listIDs(l ListResponse) string {
	ids := make([]string, 0, len(l.Incidents))
	for _, item := range l.Incidents {
		ids = append(ids, item.ID)
	}
	return strings.Join(ids, ",")
}

func TestListIsNewestFirstAndFiltersByStateAndNamespace(t *testing.T) {
	done := testIncident("c", "shop", 3*time.Minute)
	done.State = incident.Resolved
	done.Resolved = testEpoch.Add(time.Hour)
	s := newTestServer(testIncident("a", "shop", 0),
		testIncident("b", "web", time.Minute), done)

	cases := map[string]string{
		"/api/incidents":                           "b,a",
		"/api/incidents?status=all":                "c,b,a",
		"/api/incidents?status=resolved":           "c",
		"/api/incidents?status=all&namespace=shop": "c,a",
		"/api/incidents?namespace=web":             "b",
	}
	for path, want := range cases {
		w := get(t, s, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, w.Code)
		}
		if got := listIDs(decodeList(t, w.Body.String())); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
}

func TestListRejectsAnUnknownStateAndABadPage(t *testing.T) {
	s := newTestServer()
	for _, path := range []string{"/api/incidents?state=bogus",
		"/api/incidents?pageSize=0", "/api/incidents?pageSize=x",
		"/api/incidents?pageSize=10", "/api/incidents?page=0"} {
		if w := get(t, s, path); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", path, w.Code)
		}
	}
}

func TestListPageSizeKeepsTheNewestAndCountsThePages(t *testing.T) {
	items := make([]incident.Incident, 0, 120)
	for i := range 120 {
		items = append(items, testIncident("i"+strconv.Itoa(i), "ns",
			time.Duration(i)*time.Minute))
	}
	s := newTestServer(items...)

	def := decodeList(t, get(t, s, "/api/incidents").Body.String())
	small := decodeList(t, get(t, s,
		"/api/incidents?pageSize=25").Body.String())

	if len(def.Incidents) != defaultPageSize || def.Total != 120 ||
		def.Page != 1 || def.TotalPages != 3 {
		t.Errorf("default: %d items, page %d of %d, total %d",
			len(def.Incidents), def.Page, def.TotalPages, def.Total)
	}
	if !strings.HasPrefix(listIDs(small), "i119,i118,") ||
		small.PageSize != 25 || small.TotalPages != 5 {
		t.Errorf("pageSize=25: %s, %d pages", listIDs(small)[:20],
			small.TotalPages)
	}
}

func TestDetailServesTheIncidentAndAnswers404ForUnknownIDs(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Headline = "app-a in shop keeps crashing"
	p.DocsLink = "https://runbooks.example/crash"
	p.Facts = []string{"panic: nil map"}
	s := newTestServer(p)

	w := get(t, s, "/api/incidents/a")
	var got IncidentDetail
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != p.Headline || got.DocsLink != p.DocsLink ||
		len(got.CapturedLogLines) != 1 {
		t.Errorf("detail = %+v", got)
	}
	for _, path := range []string{"/api/incidents/nope",
		"/api/incidents/", "/api/incidents/a/b", "/api/other"} {
		if w := get(t, s, path); w.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, w.Code)
		}
	}
}

func TestStatusReportsScopeAndChannelHealth(t *testing.T) {
	s := newTestServer()
	scoped := newTestServer()
	scoped.opts.Scope = Scope{Watched: []string{"shop"}}

	var all, some StatusResponse
	_ = json.Unmarshal(get(t, s, "/api/status").Body.Bytes(), &all)
	_ = json.Unmarshal(get(t, scoped, "/api/status").Body.Bytes(), &some)

	if strings.Join(all.WatchedNamespaces, ",") != allNamespaces {
		t.Errorf("all namespaces = %q", all.WatchedNamespaces)
	}
	if strings.Join(some.WatchedNamespaces, ",") != "shop" {
		t.Errorf("scoped = %q", some.WatchedNamespaces)
	}
	if all.Edition != "Pro" || len(all.PerChannelHealth) != 1 ||
		all.PerChannelHealth[0].Name != "Slack" ||
		all.KwatchNamespace != "kwatch" || !all.LastScan.IsZero() {
		t.Errorf("status = %+v", all)
	}
}

func TestEveryRouteAnswers503WithoutAPipeline(t *testing.T) {
	s := newTestServer()
	s.SetIncidents(nil)
	s.SetStatus(nil)
	for _, path := range []string{"/api/incidents", "/api/incidents/a",
		"/api/status"} {
		w := get(t, s, path)
		if w.Code != http.StatusServiceUnavailable ||
			!strings.Contains(w.Body.String(), `"error":"unavailable"`) {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestEveryRouteRefusesMethodsOtherThanGet(t *testing.T) {
	s := newTestServer(testIncident("a", "shop", 0))
	for _, path := range []string{"/api/incidents", "/api/incidents/a",
		"/api/status"} {
		w := call(t, s, http.MethodPost, path, testToken)
		if w.Code != http.StatusMethodNotAllowed ||
			w.Header().Get("Allow") != http.MethodGet {
			t.Errorf("POST %s: status %d", path, w.Code)
		}
	}
}

func TestSeededSecretInALogLineNeverReachesTheAPI(t *testing.T) {
	const password = "hunter2-Sup3rS3cret"
	const token = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	p := testIncident("a", "shop", 0)
	p.Facts = []string{"connecting with DB_PASSWORD=" + password,
		"Authorization: Bearer " + token, "plain line"}
	p.Headline = "app-a fails with password=" + password
	s := newTestServer(p)

	for _, path := range []string{"/api/incidents", "/api/incidents/a"} {
		body := get(t, s, path).Body.String()
		if strings.Contains(body, password) || strings.Contains(body, token) {
			t.Errorf("%s leaked a secret: %s", path, body)
		}
	}
	if !strings.Contains(get(t, s, "/api/incidents/a").Body.String(),
		"plain line") {
		t.Error("ordinary log line missing")
	}
}
