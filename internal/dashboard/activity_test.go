package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

func before(d time.Duration) time.Time { return testEpoch.Add(-d) }

func activityLine(r ActivityResponse) string {
	parts := make([]string, 0, len(r.Events))
	for _, e := range r.Events {
		parts = append(parts, e.ID+":"+e.Kind+"@"+
			testEpoch.Sub(e.At).String())
	}
	return strings.Join(parts, " ")
}

func TestActivityDerivesNewUrgentReopenedAndResolved(t *testing.T) {
	fresh := testIncident("f", "shop", -10*time.Minute)
	fresh.Occurrences = []time.Time{fresh.Opened}
	paged := testIncident("p", "pay", -3*time.Hour)
	paged.Tier = incident.Page
	paged.Timeline = []incident.Event{
		{At: before(2 * time.Hour), Text: "still failing: paging again"},
		{At: before(5 * time.Minute), Text: "still failing: paging again"},
		{At: before(4 * time.Minute), Text: "acknowledged on x"}}
	back := testIncident("b", "web", -20*time.Minute)
	back.Occurrences = []time.Time{before(5 * time.Hour), back.Opened}
	back.History = []incident.Occurrence{{Opened: before(5 * time.Hour),
		Resolved: before(30 * time.Minute)}}
	done := testIncident("d", "web", -2*time.Hour)
	done.State, done.Resolved = incident.Resolved, before(time.Minute)
	moved := testIncident("m", "web", -2*time.Hour)
	moved.State, moved.Resolved = incident.Resolved, before(time.Minute)
	moved.SupersededBy = "f"
	quiet := testIncident("q", "web", -time.Minute)
	quiet.Tier = incident.Silent
	items := []incident.Incident{fresh, paged, back, done, moved, quiet}

	hour := buildActivity(items, testEpoch, time.Hour, redact.Evidence)
	want := "d:resolved@1m0s p:urgent@5m0s f:new@10m0s " +
		"b:reopened@20m0s b:resolved@30m0s"
	if got := activityLine(hour); got != want || hour.Total != 5 {
		t.Errorf("1h = %q (%d), want %q", got, hour.Total, want)
	}
	day := buildActivity(items, testEpoch, 24*time.Hour, redact.Evidence)
	want = want + " d:new@2h0m0s m:new@2h0m0s p:urgent@2h0m0s " +
		"p:new@3h0m0s b:new@5h0m0s"
	if got := activityLine(day); got != want {
		t.Errorf("24h = %q, want %q", got, want)
	}
	if e := hour.Events[1]; e.Title != "Deployment/pay/app-p" ||
		e.Severity != "page" || e.Namespace != "pay" ||
		e.Workload != "app-p" || e.State != "open" {
		t.Errorf("event = %+v", e)
	}
}

func TestActivityShowsARecurrenceOnce(t *testing.T) {
	first := testIncident("a1", "shop", -50*time.Minute)
	first.Occurrences = []time.Time{first.Opened}
	first.State, first.Resolved = incident.Resolved, before(40*time.Minute)
	again := first
	again.ID, again.Previous = "a2", "a1"
	again.State, again.Resolved = incident.Open, time.Time{}
	again.Opened = before(10 * time.Minute)
	again.Occurrences = []time.Time{first.Opened, again.Opened}
	again.History = []incident.Occurrence{{Opened: first.Opened,
		Resolved: first.Resolved}}
	got := activityLine(buildActivity([]incident.Incident{again, first},
		testEpoch, time.Hour, redact.Evidence))
	want := "a2:reopened@10m0s a1:resolved@40m0s a1:new@50m0s"
	if got != want {
		t.Errorf("activity = %q, want %q", got, want)
	}
}

func TestActivityKeepsTheNewestTwentyInAFixedOrder(t *testing.T) {
	items := make([]incident.Incident, 0, 30)
	for i := range 30 {
		p := testIncident("i"+strconv.Itoa(i), "ns", -time.Minute)
		p.Occurrences = []time.Time{p.Opened}
		items = append(items, p)
	}
	r := buildActivity(items, testEpoch, time.Hour, redact.Evidence)
	again := buildActivity(append(items[15:], items[:15]...), testEpoch,
		time.Hour, redact.Evidence)
	if len(r.Events) != maxActivity || r.Total != 30 ||
		activityLine(r) != activityLine(again) {
		t.Fatalf("%d of %d, stable %v", len(r.Events), r.Total,
			activityLine(r) == activityLine(again))
	}
	if r.Events[0].ID != "i0" || r.Events[1].ID != "i1" {
		t.Errorf("ties not by ID: %q", activityLine(r))
	}
}

func TestActivityHandlerValidatesSinceAndTagsTheBody(t *testing.T) {
	p := testIncident("a", "shop", -10*time.Minute)
	p.Occurrences = []time.Time{p.Opened}
	s := newTestServer(p)
	for _, path := range []string{"/api/activity?since=2h",
		"/api/activity?since=1h&since=24h", "/api/activity?since=1d"} {
		w := get(t, s, path)
		if w.Code != http.StatusBadRequest ||
			!strings.Contains(w.Body.String(), `"invalid since"`) {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := get(t, s, "/api/activity")
	var r ActivityResponse
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil ||
		w.Code != 200 || r.Since != "1h" || len(r.Events) != 1 {
		t.Fatalf("default: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "root") {
		t.Errorf("internal fields leaked: %s", w.Body.String())
	}
	tag := get(t, s, "/api/activity?since=24h").Header().Get("ETag")
	rec := getIfNoneMatch(t, s, "/api/activity?since=24h", tag)
	if tag == "" || rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match %q: %d", tag, rec.Code)
	}
}

func TestActivityNeedsAuthAndAPipeline(t *testing.T) {
	s := newTestServer()
	if w := call(t, s, http.MethodGet, "/api/activity", ""); w.Code !=
		http.StatusUnauthorized {
		t.Errorf("without a token: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, "/api/activity",
		testToken); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
	s.SetIncidents(nil)
	if w := get(t, s, "/api/activity"); w.Code !=
		http.StatusServiceUnavailable {
		t.Errorf("without a pipeline: %d", w.Code)
	}
}
