package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

const twoWeeks = 14 * 24 * time.Hour

// trendNow is a Wednesday at 12:00 UTC.
var trendNow = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func opened(id, ns string, tier incident.Tier,
	at ...time.Time) incident.Incident {
	// An open incident was announced: its opening was told.
	p := incident.Incident{ID: id, Tier: tier, State: incident.Open,
		Occurrences: at, Opened: at[len(at)-1],
		Announced: at[len(at)-1]}
	p.Root.Kind, p.Root.Namespace, p.Root.Name = "Deployment", ns, "app-"+id
	return p
}

func ago(d time.Duration) time.Time { return trendNow.Add(-d) }

func dayTotals(t TrendsResponse) string {
	out := ""
	for _, d := range t.Days {
		out += fmt.Sprintf("%s=%d ", d.Date[5:], d.Total)
	}
	return out
}

func TestTrendsBucketsDaysInUTCAndInTheGivenZone(t *testing.T) {
	// 23:30 UTC on 27 Oct is 28 Oct in UTC+1 and still 27 Oct in UTC.
	late := time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC)
	items := []incident.Incident{
		opened("a", "shop", incident.Page, late),
		opened("b", "shop", incident.Notify, ago(6*24*time.Hour)),
		opened("c", "web", incident.Digest, ago(7*24*time.Hour)),
	}
	utc := buildTrends(items, trendNow, time.UTC, 0)
	want := "10-22=1 10-23=0 10-24=0 10-25=0 10-26=0 10-27=1 10-28=0 "
	if got := dayTotals(utc); got != want {
		t.Errorf("UTC days = %q, want %q", got, want)
	}
	if utc.Opened != 2 || utc.Days[5].Page != 1 || utc.Days[0].Notify != 1 {
		t.Errorf("UTC = %+v", utc)
	}
	plus := buildTrends(items, trendNow, offsetZone(60), 0)
	want = "10-22=1 10-23=0 10-24=0 10-25=0 10-26=0 10-27=0 10-28=1 "
	if got := dayTotals(plus); got != want {
		t.Errorf("UTC+1 days = %q, want %q", got, want)
	}
	// In UTC-13 it is 27 Oct 23:00: c (21 Oct 12:00 UTC) is
	// 20 Oct there, before the week of 21 to 27 Oct.
	minus := buildTrends(items, trendNow, offsetZone(-13*60), 0)
	if minus.Days[0].Date != "2026-10-21" || minus.Opened != 2 {
		t.Errorf("UTC-13 = %q, %d opened", dayTotals(minus), minus.Opened)
	}
}

func TestTrendsEmptyDataIsSevenZeroDaysAndNotEnough(t *testing.T) {
	got := buildTrends(nil, trendNow, time.UTC, twoWeeks)
	if len(got.Days) != trendDays || got.Opened != 0 || got.Enough ||
		got.PreviousWeek != nil || len(got.Namespaces) != 0 ||
		len(got.Noisiest) != 0 || got.Resolve.Count != 0 {
		t.Errorf("empty trends = %+v", got)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	_ = json.Unmarshal(body, &back)
	if back["namespaces"] == nil || back["noisiest"] == nil {
		t.Errorf("empty lists must be [] not null: %s", body)
	}
}

func TestTrendsLeavesOutSilentIssues(t *testing.T) {
	got := buildTrends([]incident.Incident{
		opened("q", "shop", incident.Silent, ago(time.Hour), ago(0)),
	}, trendNow, time.UTC, 0)
	if got.Opened != 0 || len(got.Noisiest) != 0 {
		t.Errorf("silent counted: %+v", got)
	}
}

func TestTrendsPreviousWeekOnlyWhenKeptLongEnough(t *testing.T) {
	items := []incident.Incident{
		opened("a", "shop", incident.Notify, ago(10*24*time.Hour),
			ago(time.Hour)),
		opened("b", "shop", incident.Notify, ago(15*24*time.Hour)),
	}
	if got := buildTrends(items, trendNow, time.UTC,
		incident.DefaultRemember); got.PreviousWeek != nil {
		t.Errorf("a week of retention compared with %d",
			*got.PreviousWeek)
	}
	got := buildTrends(items, trendNow, time.UTC, twoWeeks)
	if got.PreviousWeek == nil || *got.PreviousWeek != 1 {
		t.Fatalf("previous week = %v, want 1", got.PreviousWeek)
	}
	// Nothing kept from before last week: kwatch may not have run then.
	got = buildTrends(items[:1], trendNow, time.UTC, twoWeeks)
	if got.PreviousWeek != nil {
		t.Errorf("uncovered week compared with %d", *got.PreviousWeek)
	}
}

func TestTrendsNamespacesAreTopFiveByOpenings(t *testing.T) {
	var items []incident.Incident
	for i, ns := range []string{"a", "b", "c", "d", "e", "f"} {
		var at []time.Time
		for range i + 1 {
			at = append(at, ago(time.Hour))
		}
		items = append(items, opened(ns, ns, incident.Notify, at...))
	}
	done := opened("g", "a", incident.Notify, ago(time.Hour))
	done.State = incident.Resolved
	cluster := opened("n", "", incident.Page, ago(time.Hour))
	got := buildTrends(append(items, done, cluster), trendNow, time.UTC, 0)
	want := "f6/1 e5/1 d4/1 c3/1 a2/1 "
	s := ""
	for _, n := range got.Namespaces {
		s += fmt.Sprintf("%s%d/%d ", n.Namespace, n.Count, n.Active)
	}
	if s != want {
		t.Errorf("namespaces = %q, want %q", s, want)
	}
}

func TestTrendsNoisiestOrdersRepeatsThenFlappingAndIsBounded(
	t *testing.T,
) {
	flap := opened("flap", "a", incident.Notify, ago(time.Hour))
	flap.State = incident.Flapping
	twice := opened("twice", "a", incident.Notify, ago(2*time.Hour),
		ago(time.Hour))
	twice.RepeatCount = 2
	twiceFlap := opened("twice-flap", "a", incident.Notify,
		ago(2*time.Hour), ago(time.Hour))
	twiceFlap.State = incident.Flapping
	old := opened("old", "a", incident.Notify, ago(9*24*time.Hour),
		ago(8*24*time.Hour), ago(time.Hour))
	items := []incident.Incident{flap, twice, twiceFlap, old,
		opened("once", "a", incident.Notify, ago(time.Hour))}
	for i := range 5 {
		items = append(items, opened(fmt.Sprintf("z%d", i), "a",
			incident.Page, ago(3*time.Hour), ago(2*time.Hour),
			ago(time.Hour)))
	}
	got := buildTrends(items[:5], trendNow, time.UTC, 0)
	ids := ""
	for _, n := range got.Noisiest {
		ids += fmt.Sprintf("%s:%d ", n.ID, n.Opened)
	}
	if want := "twice-flap:2 twice:2 flap:1 "; ids != want {
		t.Errorf("noisiest = %q, want %q", ids, want)
	}
	if got = buildTrends(items, trendNow, time.UTC, 0); len(got.Noisiest) !=
		trendTop || got.Noisiest[0].ID != "z0" {
		t.Errorf("bounded noisiest = %+v", got.Noisiest)
	}
}

func TestTrendsTimeToResolveMedianAndLongest(t *testing.T) {
	p := opened("a", "shop", incident.Notify, ago(5*time.Hour))
	p.State, p.Opened, p.Resolved = incident.Resolved, ago(5*time.Hour),
		ago(4*time.Hour)
	p.History = []incident.Occurrence{
		{Opened: ago(30 * time.Hour), Resolved: ago(20 * time.Hour)},
		// Resolved before the week: left out.
		{Opened: ago(9 * 24 * time.Hour), Resolved: ago(8 * 24 * time.Hour)},
	}
	q := opened("b", "web", incident.Page, ago(3*time.Hour))
	q.History = []incident.Occurrence{
		{Opened: ago(3 * time.Hour), Resolved: ago(150 * time.Minute)},
	}
	got := buildTrends([]incident.Incident{p, q}, trendNow, time.UTC, 0)
	want := TrendResolve{Count: 3, MedianSeconds: 3600,
		LongestSeconds: 36000, LongestID: "a"}
	if got.Resolve != want {
		t.Errorf("resolve = %+v, want %+v", got.Resolve, want)
	}
	two := buildTrends([]incident.Incident{p}, trendNow, time.UTC, 0)
	if two.Resolve.MedianSeconds != (3600+36000)/2 {
		t.Errorf("even median = %d", two.Resolve.MedianSeconds)
	}
}

func TestOverviewTrendsValidateTZAndUseTheClock(t *testing.T) {
	s := newTestServer(testIncident("a", "shop", -time.Hour),
		testIncident("b", "shop", -2*time.Hour))
	for _, path := range []string{"/api/overview?tz=x",
		"/api/overview?tz=841", "/api/overview?tz=-841"} {
		if w := get(t, s, path); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, w.Code)
		}
	}
	w := getPart(t, s, "/api/overview?tz=-840", "trends")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var out TrendsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Opened != 2 || !out.Enough ||
		out.Days[trendDays-1].Date != "2026-09-30" {
		t.Errorf("trends = %+v", out)
	}
	if w := call(t, s, http.MethodGet, "/api/overview", ""); w.Code !=
		http.StatusUnauthorized {
		t.Errorf("no token: %d", w.Code)
	}
	if w := call(t, s, http.MethodPost, "/api/overview",
		testToken); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
	s.SetIncidents(nil)
	if w := get(t, s, "/api/overview"); w.Code !=
		http.StatusServiceUnavailable {
		t.Errorf("no source: %d", w.Code)
	}
}

func TestTrendsAnswerNotModifiedForTheSameETag(t *testing.T) {
	s := newTestServer(testIncident("a", "shop", -time.Hour))
	first := get(t, s, "/api/overview")
	tag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || tag == "" {
		t.Fatalf("first: %d, ETag %q", first.Code, tag)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/overview", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("If-None-Match", tag)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotModified {
		t.Errorf("same ETag: %d, want 304", w.Code)
	}
}
