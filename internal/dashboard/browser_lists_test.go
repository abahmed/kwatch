//go:build browser

package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// listsServer serves the dashboard with some /api/* paths answered by
// fixed bodies, so the Overview and the Issues list are drawn from
// answers a test chooses.
func listsServer(t *testing.T, answers map[string]any) *httptest.Server {
	t.Helper()
	bodies := map[string][]byte{}
	for path, v := range answers {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		bodies[path] = b
	}
	inner := newTestServer(browserIssues()...).Handler()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if b, ok := bodies[r.URL.Path]; ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(b)
				return
			}
			inner.ServeHTTP(w, r)
		}))
	t.Cleanup(srv.Close)
	return srv
}

func listAgo(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format(time.RFC3339)
}

// overviewAnswer is an /api/overview answer with the given counts, a
// release that failed (with its issue), another that went down with no
// issue, and an issue that started after a change (the failed
// release's).
func overviewAnswer(urgent, attention, low int, bad bool) map[string]any {
	days := []map[string]any{}
	for i := 0; i < 7; i++ {
		days = append(days, map[string]any{"date": fmt.Sprintf(
			"2026-11-%02d", i+3), "opened": 10, "resolved": 9,
			"paged": 1, "untold": 7})
	}
	ins := map[string]any{"days": days, "causes": []any{},
		"nodes": []any{}, "namespaces": map[string]any{
			"tiles": []any{}, "more": 0}}
	rel := map[string]any{"total": 3, "bad": 0}
	if bad {
		rel = map[string]any{"total": 3, "bad": 2, "wrong": []any{
			map[string]any{"name": "api", "namespace": "shop",
				"kind": "deployment", "at": listAgo(2 * time.Hour),
				"verdict": "failed", "incident": "inc-001"},
			map[string]any{"name": "worker", "namespace": "shop",
				"kind": "deployment", "at": listAgo(3 * time.Hour),
				"verdict": "down"}}}
		ins["afterChange"] = map[string]any{"count": 1, "total": 5,
			"issues": []any{map[string]any{"id": "inc-001",
				"title":    "api in shop: the new version can't start",
				"severity": "notify", "state": "open",
				"namespace": "shop", "workload": "api",
				"kind": "deployment", "since": listAgo(110 * time.Minute),
				"object": "deployment/api", "change": "image a → b",
				"changedAt": listAgo(2 * time.Hour)}}}
	}
	look := []any{}
	if urgent+attention+low > 0 {
		look = append(look, map[string]any{"id": "inc-001",
			"title": "api in shop can't run: its image is not built " +
				"for this node's CPU", "namespace": "shop",
			"kind": "deployment", "name": "api", "severity": "page",
			"state": "open", "since": listAgo(6 * time.Hour)})
	}
	return map[string]any{"counts": map[string]any{"urgent": urgent,
		"attention": attention, "low": low, "quiet": 0},
		"lookFirst": look, "releases": rel, "insights": ins,
		"trends": map[string]any{"days": []any{}, "opened": 70,
			"namespaces": []any{}, "noisiest": []any{},
			"resolve": map[string]any{"count": 5, "medianSeconds": 8},
			"enough":  true}}
}

// The Overview's status line says the counts by level, and is red only
// while something is urgent.
func TestBrowserOverviewStatusLine(t *testing.T) {
	actx, _ := browserSetup(t)
	cases := []struct {
		name                    string
		urgent, attention, low  int
		class, text, notClasses string
	}{
		{"urgent", 1, 10, 29, "bad",
			"1 urgent · 10 need attention · 29 low", "warn"},
		{"attention", 0, 2, 3, "warn", "2 need attention · 3 low", "bad"},
		{"low", 0, 0, 4, "ok", "4 low", "bad warn"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := listsServer(t, map[string]any{"/api/overview": overviewAnswer(
				c.urgent, c.attention, c.low, false)})
			tab, _ := browserTab(t, actx)
			signInBrowser(t, tab, srv.URL)
			err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 800),
				chromedp.Navigate(srv.URL+"/"),
				chromedp.WaitVisible(chromedp.CSS("#ov-say a")))
			if err != nil {
				t.Fatal(err)
			}
			got, err := chromedp.Run(tab, chromedp.Evaluate[string](`
  document.querySelector("#ov-say").textContent + "|" +
  document.querySelector(".ov-status").className + "|" +
  document.querySelectorAll(".ov-sev").length`))
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(got, "|")
			if parts[0] != c.text {
				t.Errorf("status line is %q, want %q", parts[0], c.text)
			}
			if !strings.Contains(" "+parts[1]+" ", " "+c.class+" ") {
				t.Errorf("tile is %q, want %q", parts[1], c.class)
			}
			for _, bad := range strings.Fields(c.notClasses) {
				if strings.Contains(" "+parts[1]+" ", " "+bad+" ") {
					t.Errorf("tile is %q, not %q", parts[1], bad)
				}
			}
			// The count bar and its legend repeat the line: gone.
			if parts[2] != "0" {
				t.Errorf("%s count bars repeat the status line", parts[2])
			}
		})
	}
}

// Bad releases and changes that caused issues are one block: the
// failed release and the issue that followed it are one line, a release
// with no issue is another, and nothing shows when all is well.
func TestBrowserOverviewMergesReleasesAndChanges(t *testing.T) {
	actx, _ := browserSetup(t)
	srv := listsServer(t, map[string]any{
		"/api/overview": overviewAnswer(1, 0, 0, true)})
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS(".ov-rel li")))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](`
  document.querySelector(".ov-rel .ov-h2").textContent + "|" +
  document.querySelectorAll(".ov-rel li").length + "|" +
  [...document.querySelectorAll(".ov-rel li")].map((l) => l.textContent).
    join(" / ") + "|" +
  document.body.textContent.includes("Recent bad releases") + "|" +
  document.body.textContent.includes("Started right after a change")`))
	if err != nil {
		t.Fatal(err)
	}
	p := strings.SplitN(got, "|", 5)
	if p[0] != "Bad releases and changes that caused issues" || p[1] != "2" {
		t.Errorf("block is %q with %s rows", p[0], p[1])
	}
	for _, want := range []string{"api: the new version can't start",
		"The release failed", "image a → b", "worker in shop",
		"No issue was opened"} {
		if !strings.Contains(p[2], want) {
			t.Errorf("block lacks %q: %q", want, p[2])
		}
	}
	if p[3] != "false" || p[4] != "false" {
		t.Errorf("the old blocks are still there: %s %s", p[3], p[4])
	}

	calm := listsServer(t, map[string]any{
		"/api/overview": overviewAnswer(1, 0, 0, false)})
	tab, _ = browserTab(t, actx)
	signInBrowser(t, tab, calm.URL)
	err = chromedp.Do(tab, chromedp.Navigate(calm.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("#ov-say a")))
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := chromedp.Run(tab, chromedp.Evaluate[bool](
		`document.querySelector(".ov-rel").hidden`))
	if err != nil || !hidden {
		t.Errorf("an empty releases block shows (%v, %v)", hidden, err)
	}
}

// The week says in a plain line how many issues settled on their own.
func TestBrowserOverviewWeekSaysTheNoise(t *testing.T) {
	actx, _ := browserSetup(t)
	srv := listsServer(t, map[string]any{
		"/api/overview": overviewAnswer(1, 0, 0, false)})
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS(".ov-settled")))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](`
  document.querySelector(".ov-settled").textContent + "|" +
  [...document.querySelectorAll(".ov-stat dt")].map((d) => d.textContent).
    join(",")`))
	if err != nil {
		t.Fatal(err)
	}
	want := "49 of 70 issues settled on their own before anyone was " +
		"alerted.|Alerted,Resolved,Alerted the on-call person," +
		"Median time to resolve an alerted issue"
	if got != want {
		t.Errorf("week says %q, want %q", got, want)
	}
}

// listItems are issues with long names, repeats and an old one, as a
// busy cluster has them.
func listItems(resolved bool) []any {
	var out []any
	for i := 0; i < 8; i++ {
		it := map[string]any{"id": fmt.Sprintf("inc-%03d", i),
			"title": "Mutating webhook actions-runner-controller-" +
				"mutating-webhook-configuration is failing",
			"severity": []string{"page", "notify", "digest"}[i%3],
			"state":    "open", "namespace": "external-secrets",
			"workload":     "external-secrets-cert-controller-69b44696db",
			"workloadKind": "deployment", "repeatCount": i % 3,
			"firstSeen": listAgo(time.Duration(i+1) * 7 * time.Hour),
			"lastSeen":  listAgo(time.Hour), "rev": fmt.Sprint(i)}
		if resolved && i%2 == 1 {
			it["state"] = "resolved"
			it["resolvedAt"] = listAgo(time.Hour)
			it["untold"] = true
		}
		out = append(out, it)
	}
	return out
}

func listAnswer(resolved bool) map[string]any {
	return map[string]any{"incidents": listItems(resolved), "total": 8,
		"page": 1, "pageSize": 50, "totalPages": 1,
		"facets": map[string]any{
			"severity":  map[string]any{"page": 3, "notify": 3, "digest": 2},
			"state":     map[string]any{"open": 8},
			"namespace": map[string]any{"external-secrets": 8},
			"kind":      map[string]any{"deployment": 8},
			"acked":     map[string]any{"false": 8},
			"stopped":   map[string]any{"false": 8}}}
}

// The Issues toolbar is one row (status, search, Filters) with the rest
// behind the button; Active has no empty Status column; a repeat reads
// "failed again 2 times".
func TestBrowserIssuesToolbarIsOneRow(t *testing.T) {
	actx, _ := browserSetup(t)
	srv := listsServer(t, map[string]any{
		"/api/incidents": listAnswer(false)})
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(srv.URL+"/issues"),
		chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](`(() => {
  const top = (s) => Math.round(document.querySelector(s).
    getBoundingClientRect().top / 8);
  const rows = new Set([top(".stsw"), top(".f-search"), top("[data-ftoggle]")]);
  const panel = getComputedStyle(
    document.querySelector("[data-fmore]")).display;
  const table = Math.round(document.querySelector("#issues-table").
    getBoundingClientRect().top);
  return [rows.size, panel, table,
    document.querySelectorAll("th.c-status").length,
    document.querySelector("[data-ftoggle]").textContent,
    [...document.querySelectorAll(".i-rep")].map((e) => e.textContent).
      join(",")].join("|");
})()`))
	if err != nil {
		t.Fatal(err)
	}
	p := strings.Split(got, "|")
	if p[0] != "1" {
		t.Errorf("status, search and Filters are on %s rows", p[0])
	}
	if p[1] != "none" {
		t.Errorf("the other filters show before Filters is pressed")
	}
	var table int
	_, _ = fmt.Sscan(p[2], &table)
	if table > 180 {
		t.Errorf("the table starts %dpx down", table)
	}
	if p[3] != "0" {
		t.Errorf("Active has a Status column")
	}
	if p[4] != "Filters" {
		t.Errorf("button says %q", p[4])
	}
	if !strings.Contains(p[5], "failed again 2 times") ||
		strings.Contains(p[5], "×") {
		t.Errorf("repeats read %q", p[5])
	}
	// The button opens severity, quick views and the view buttons.
	opened, err := chromedp.Run(tab, chromedp.Evaluate[string](`(() => {
  document.querySelector("[data-ftoggle]").click();
  const p = document.querySelector("[data-fmore]");
  return getComputedStyle(p).display + "|" +
    !!p.querySelector(".fchip") + "|" + !!p.querySelector("[data-qv]") +
    "|" + !!p.querySelector("[data-vopts]");
})()`))
	if err != nil || opened != "flex|true|true|true" {
		t.Errorf("the panel is %q (%v)", opened, err)
	}
}

// On a phone the Issues list never scrolls sideways, on Active or All,
// and each issue is a compact card.
func TestBrowserIssuesPhoneCards(t *testing.T) {
	actx, _ := browserSetup(t)
	for _, status := range []string{"active", "all"} {
		srv := listsServer(t, map[string]any{
			"/api/incidents": listAnswer(status == "all")})
		tab, _ := browserTab(t, actx)
		signInBrowser(t, tab, srv.URL)
		err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
			chromedp.Navigate(srv.URL+"/issues?status="+status),
			chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")),
			chromedp.Sleep(500*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		over, err := chromedp.Run(tab, chromedp.Evaluate[float64](
			overflowScript))
		if err != nil || over > 1 {
			t.Errorf("%s: scrolls sideways by %.0fpx (%v)", status, over, err)
		}
		tall, err := chromedp.Run(tab, chromedp.Evaluate[float64](`Math.max(
  ...[...document.querySelectorAll("#issues-table tbody tr")].
    map((r) => r.getBoundingClientRect().height))`))
		if err != nil || tall >= 175 {
			t.Errorf("%s: a card is %.0fpx tall (%v)", status, tall, err)
		}
	}
	// A short title and name: two lines, under 110px.
	srv := listsServer(t, nil)
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(srv.URL+"/issues"),
		chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")))
	if err != nil {
		t.Fatal(err)
	}
	tall, err := chromedp.Run(tab, chromedp.Evaluate[float64](`Math.max(
  ...[...document.querySelectorAll("#issues-table tbody tr")].
    map((r) => r.getBoundingClientRect().height))`))
	if err != nil || tall >= 110 {
		t.Errorf("a card is %.0fpx tall (%v)", tall, err)
	}
}
