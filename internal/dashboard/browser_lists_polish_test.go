//go:build browser

package dashboard

import (
	"fmt"
	"testing"
	"time"
)

// A row that settled on its own shows its level greyed on one line,
// never "Was ...", on the Issues list (beside "Settled on its own")
// and on Activity; "Was Urgent" is kept for a resolved issue people
// were alerted about.
func TestBrowserSettledRowsShowTheirLevelGreyed(t *testing.T) {
	ev := map[string]any{"id": "inc-003", "at": listAgo(10 * time.Minute),
		"kind": "resolved", "severity": "page", "untold": true,
		"title": "api in shop is not ready", "workload": "api",
		"namespace": "shop"}
	told := map[string]any{"id": "inc-004", "at": listAgo(time.Hour),
		"kind": "resolved", "severity": "page",
		"title": "web in shop is not ready", "workload": "web",
		"namespace": "shop"}
	srv := listsServer(t, map[string]any{
		"/api/incidents": listAnswer(true),
		"/api/activity": map[string]any{"events": []any{ev, told},
			"total": 2, "kinds": map[string]int{"resolved": 2},
			"untold": 0}})
	probe := `(() => { const c = document.querySelector(".c-settled");
	  const r = c.getBoundingClientRect();
	  const lh = parseFloat(getComputedStyle(c).lineHeight) || 20;
	  const was = [...document.querySelectorAll(".chip")].
	    map((x) => x.textContent).filter((x) => /^Was/.test(x));
	  return c.textContent + "|" + c.className + "|" +
	    (r.height < 2 * lh) + "|" + was.join(",") + "|" +
	    !!document.querySelector(".m-untold"); })()`
	for path, want := range map[string]string{
		"/issues?status=resolved": "Needs attention|chip c-acked " +
			"c-settled|true||true",
		"/activity": "Urgent|chip c-acked c-settled|true|Was Urgent|" +
			"false"} {
		for _, w := range []int64{1280, 390} {
			text, done := polishPage(t, srv.URL, path, ".c-settled", w)
			if got := text(probe); got != want {
				t.Errorf("%s at %d: %q, want %q", path, w, got, want)
			}
			done()
		}
	}
}

// Between 900 and 1100px the Where column is gone and the namespace
// sits under the title; elsewhere the line is not drawn twice.
func TestBrowserIssuesKeepNamespaceAtMidWidth(t *testing.T) {
	srv := listsServer(t, map[string]any{
		"/api/incidents": listAnswer(false)})
	probe := `(() => { const n = document.querySelector(".it-ns");
	  const w = document.querySelector("td.c-where");
	  return getComputedStyle(n).display + "|" + n.textContent + "|" +
	    getComputedStyle(w).display; })()`
	text, done := polishPage(t, srv.URL, "/issues",
		"#issues-table tbody tr", 1000)
	defer done()
	if got := text(probe); got != "block|external-secrets|none" {
		t.Errorf("1000px: %q", got)
	}
	// And the title is balanced across its lines, never split inside a
	// word before it has to be.
	if got := text(`getComputedStyle(document.querySelector(
	  ".it-link")).overflowWrap`); got != "break-word" {
		t.Errorf("title breaks inside words: %q", got)
	}
	text2, done2 := polishPage(t, srv.URL, "/issues",
		"#issues-table tbody tr", 1280)
	defer done2()
	if got := text2(probe); got != "none|external-secrets|table-cell" {
		t.Errorf("1280px: %q", got)
	}
}

// Activity's time column keeps a stamp on one line, even for an older
// day ("1 Oct, 15:00").
func TestBrowserActivityTimeStaysOnOneLine(t *testing.T) {
	var evs []any
	for i := 0; i < 3; i++ {
		evs = append(evs, map[string]any{"id": fmt.Sprint("inc-", i),
			"at":   listAgo(time.Duration(40+i) * 24 * time.Hour),
			"kind": "new", "severity": "notify", "workload": "api",
			"title": "api in shop is not ready", "namespace": "shop"})
	}
	srv := listsServer(t, map[string]any{"/api/activity": map[string]any{
		"events": evs, "total": 3, "kinds": map[string]int{"new": 3},
		"untold": 0}})
	for _, w := range []int64{1280, 390} {
		text, done := polishPage(t, srv.URL, "/activity?since=24h",
			".act-at", w)
		got := text(`(() => { const a = document.querySelector(".act-at");
		  return Math.round(a.getBoundingClientRect().height /
		    parseFloat(getComputedStyle(a).lineHeight)) + "|" + a.textContent;
		})()`)
		if got[:2] != "1|" {
			t.Errorf("%dpx: time wraps: %q", w, got)
		}
		done()
	}
}
