//go:build browser

package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// realAnswer is the server's own JSON answer at path, as a map the
// browser fixtures change.
func realAnswer(t *testing.T, path string) map[string]any {
	t.Helper()
	w := get(t, newTestServer(browserIssues()...), path)
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

// leftoverObjects are n objects a removed controller left behind.
func leftoverObjects(n int) []any {
	var out []any
	for i := 0; i < n; i++ {
		out = append(out, map[string]any{"kind": "Secret",
			"namespace": "shop", "name": fmt.Sprintf("web-tls-%02d", i),
			"reason": "TLSCertExpired",
			"what":   "TLS certificate expired on 2025-01-01"})
	}
	return out
}

// lowered is the reason kwatch gives for an old problem it lowered.
const lowered = "Lowered to Low: already failing for about 2 years " +
	"when kwatch first saw it, and nothing has changed since. " +
	"kwatch lists it in the daily summary and sends no alert."

// leftoverDetail is inc-003 as a group of 12 left-behind objects that
// kwatch lowered to Low.
func leftoverDetail(t *testing.T) map[string]any {
	d := realAnswer(t, "/api/incidents/inc-003")
	d["leftoverObjects"] = 12
	d["leftovers"] = leftoverObjects(12)
	d["severity"] = "digest"
	d["why"] = map[string]any{"alertLevelReason": lowered}
	return d
}

func pagingTab(t *testing.T, srv *httptest.Server, w, h int64,
	path, ready string) context.Context {
	t.Helper()
	actx, _ := browserSetup(t)
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(w, h),
		chromedp.Navigate(srv.URL+path),
		chromedp.WaitVisible(chromedp.CSS(ready)),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

func evalString(t *testing.T, tab context.Context, script string) string {
	t.Helper()
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](script))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// D1: a leftover group's row says how many objects it holds, and the
// phone card stays small.
func TestBrowserLeftoverGroupRow(t *testing.T) {
	list := listAnswer(true)
	items := list["incidents"].([]any)
	items[0].(map[string]any)["leftoverObjects"] = 12
	// Three marks and a long namespace: the card still fits.
	busy := items[1].(map[string]any)
	busy["leftoverObjects"] = 3
	busy["namespace"] = "external-secrets-operator-system-" +
		"long-namespace-name"
	srv := listsServer(t, map[string]any{"/api/incidents": list})
	for _, size := range browserSizes {
		tab := pagingTab(t, srv, size.w, size.h, "/issues?status=all",
			"#issues-table tbody tr")
		got := evalString(t, tab, `[...document.querySelectorAll(
  "#issues-table .i-left")].map((c) => c.textContent).join(",") + "|" +
  Math.max(...[...document.querySelectorAll("#issues-table tbody tr")].
    map((r) => r.getBoundingClientRect().height)) + "|" +
  document.querySelector("#issues-table tbody tr").
    getBoundingClientRect().height`)
		p := strings.Split(got, "|")
		if p[0] != "12 objects,3 objects" {
			t.Errorf("%dpx: count chips are %q", size.w, p[0])
		}
		var tall float64
		_, _ = fmt.Sscan(p[1], &tall)
		if size.w < 700 && tall >= 175 {
			t.Errorf("a phone card is %.0fpx tall", tall)
		}
		// The chip ends the card's last line: it adds no line.
		var lone float64
		_, _ = fmt.Sscan(p[2], &lone)
		if size.w < 700 && lone >= 160 {
			t.Errorf("the card with only a chip is %.0fpx tall", lone)
		}
	}
}

// D1 and D3: the issue page lists the first ten objects with "Show all
// 12", "What's wrong?" opens them, and the level says why it is Low.
func TestBrowserLeftoverGroupIssuePage(t *testing.T) {
	srv := listsServer(t, map[string]any{
		"/api/incidents/inc-003": leftoverDetail(t)})
	tab := pagingTab(t, srv, 1280, 800, "/issues/inc-003#timeline",
		"[data-level-note]")
	got := evalString(t, tab, `(() => {
  const q = (s) => document.querySelector(s);
  const line = q(".lo-line").textContent;
  q(".lo-line button").click();
  const sec = q('[data-fold="leftovers"]');
  const shown = () => [...sec.querySelectorAll(".lo-row")].
    filter((r) => !r.hidden).length;
  const before = shown(), all = q(".lo-all").textContent;
  q(".lo-all").click();
  return [q("[data-level-note]").textContent, line,
    sec.querySelector(".sec-h").textContent, !sec.closest("[hidden]"),
    before, all, shown(),
    sec.querySelector(".lo-row").textContent].join("|");
})()`)
	want := "Lowered to Low: already failing for about 2 years when " +
		"kwatch first saw it, and nothing has changed since|" +
		"12 objects left behind · See which →|" +
		"Objects left behind (12)|true|10|Show all 12|12|" +
		"Secret · shop/web-tls-00TLS certificate expired on 2025-01-01"
	if got != want {
		t.Errorf("issue page says\n%q, want\n%q", got, want)
	}
}

// D3: a page held while the cluster scales says so beside the level,
// and an ordinary level reason adds nothing there.
func TestBrowserLevelNoteHeldAndPlain(t *testing.T) {
	held := realAnswer(t, "/api/incidents/inc-004")
	held["state"] = "settling"
	held["why"] = map[string]any{"alertLevelReason": "Urgent: users " +
		"can't reach it.", "pagingHeldReason": "Paging held: cluster " +
		"is scaling"}
	plain := realAnswer(t, "/api/incidents/inc-005")
	plain["why"] = map[string]any{"alertLevelReason": "Needs attention: " +
		"something failed, but users are still served."}
	srv := listsServer(t, map[string]any{"/api/incidents/inc-004": held,
		"/api/incidents/inc-005": plain})
	tab := pagingTab(t, srv, 1280, 800, "/issues/inc-004",
		"[data-level-note]")
	if got := evalString(t, tab, `document.querySelector(
  "[data-level-note]").textContent`); got != "Paging held: cluster is scaling" {
		t.Errorf("held note is %q", got)
	}
	tab = pagingTab(t, srv, 1280, 800, "/issues/inc-005", "#d-title")
	if got := evalString(t, tab, `String(document.querySelectorAll(
  "[data-level-note]").length)`); got != "0" {
		t.Errorf("a plain level shows %s notes", got)
	}
}

// overviewPaging is the Overview with old problems and a scaling hold.
func overviewPaging(old int) map[string]any {
	ov := overviewAnswer(1, 2, 0, false)
	var items []any
	for i := 0; i < old; i++ {
		items = append(items, map[string]any{"kind": "Job",
			"namespace": "shop", "name": fmt.Sprintf("report-%02d", i),
			"reason": "JobFailed", "what": "The job failed",
			"since": listAgo(640 * 24 * time.Hour), "ageDays": 640})
	}
	ov["oldProblems"] = map[string]any{"count": old, "days": 30,
		"items": items}
	end := time.Now().Add(20 * time.Minute).UTC()
	ov["scalingHold"] = map[string]any{"reason": "Paging held: " +
		"cluster is scaling", "since": listAgo(5 * time.Minute),
		"expectedEnd": end.Format(time.RFC3339)}
	return ov
}

// D2 and D4: one quiet line counts the old problems and opens them;
// the status tile says paging is held until about when.
func TestBrowserOverviewOldProblemsAndHold(t *testing.T) {
	srv := listsServer(t, map[string]any{
		"/api/overview": overviewPaging(3)})
	tab := pagingTab(t, srv, 1280, 800, "/", ".ov-hold")
	got := evalString(t, tab, `(() => {
  const q = (s) => document.querySelector(s);
  const closed = !q(".ov-old .lo-row").checkVisibility();
  q(".ov-old summary").click();
  return [q(".ov-old summary").textContent, closed,
    q(".ov-old .lo-row").checkVisibility(),
    q(".ov-old .lo-row").textContent, q(".ov-hold").textContent].join("|");
})()`)
	p := strings.Split(got, "|")
	want := []string{"3 old problems not shown (already failing for 30+ " +
		"days when kwatch first saw them) Show them", "true", "true",
		"Job · shop/report-00The job failedfor about 2 years"}
	for i, w := range want {
		if p[i] != w {
			t.Errorf("part %d is %q, want %q", i, p[i], w)
		}
	}
	if !strings.HasPrefix(p[4], "Paging held while the cluster scales"+
		" · until ~") || len(p[4]) < 45 {
		t.Errorf("hold line is %q", p[4])
	}

	none := listsServer(t, map[string]any{
		"/api/overview": overviewAnswer(1, 2, 0, false)})
	tab = pagingTab(t, none, 1280, 800, "/", "#ov-say a")
	if got := evalString(t, tab, `String(document.querySelector(
  ".ov-olds").hidden) +
  document.querySelectorAll(".ov-hold").length`); got != "true0" {
		t.Errorf("without old problems or a hold the page shows %q", got)
	}
}

// D5: the health page says a non-production cluster pages nobody,
// and says nothing of it otherwise.
func TestBrowserHealthNonProduction(t *testing.T) {
	st := realAnswer(t, "/api/status")
	k, ok := st["kwatch"].(map[string]any)
	if !ok {
		t.Fatal("status has no kwatch part")
	}
	k["nonProduction"] = true
	srv := listsServer(t, map[string]any{"/api/status": st})
	tab := pagingTab(t, srv, 390, 844, "/kwatch", ".ov-say")
	if got := evalString(t, tab, `(document.querySelector(".kp-np") ||
  {}).textContent || ""`); got != "Non-production cluster: nothing pages" {
		t.Errorf("non-production line is %q", got)
	}
	k["nonProduction"] = false
	srv = listsServer(t, map[string]any{"/api/status": st})
	tab = pagingTab(t, srv, 1280, 800, "/kwatch", ".ov-say")
	if got := evalString(t, tab, `String(document.querySelectorAll(
  ".kp-np").length)`); got != "0" {
		t.Errorf("a production cluster shows %s non-production lines", got)
	}
}
