//go:build browser

package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// refreshPage makes the page read again, as its poll does, and waits
// for the answer to be drawn.
func refreshPage(t *testing.T, tab context.Context) {
	t.Helper()
	evalString(t, tab, `(() => {
  document.dispatchEvent(new Event("visibilitychange"));
  return "";
})()`)
	if err := chromedp.Do(tab,
		chromedp.Sleep(700*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
}

// shownRows counts the rows of the list under sel that are not hidden,
// then whether "Show all" is offered.
const shownRows = `((sel) => {
  const box = document.querySelector(sel);
  return [...box.querySelectorAll(".lo-row")].
    filter((r) => !r.hidden).length + "|" +
    box.querySelectorAll(".lo-all").length;
})`

// The Overview's old problems are all shown, never behind a fold or a
// "Show all", and stay so after the page reads again.
func TestBrowserOldProblemsAreAllShown(t *testing.T) {
	srv := listsServer(t, map[string]any{
		"/api/overview": overviewPaging(20)})
	tab := pagingTab(t, srv, 1280, 800, "/", ".ov-old")
	probe := shownRows + `(".ov-old") + "," +
  document.querySelectorAll(".ov-olds details").length`
	if before := evalString(t, tab, probe); before != "20|0,0" {
		t.Fatalf("before the refresh the list shows %q", before)
	}
	refreshPage(t, tab)
	if after := evalString(t, tab, probe); after != "20|0,0" {
		t.Errorf("after a refresh the list shows %q, want all 20", after)
	}
}

// changingDetail serves inc-003 as a leftover group whose first object
// reads differently on every answer, so each read redraws the issue.
func changingDetail(t *testing.T) *httptest.Server {
	t.Helper()
	d := leftoverDetail(t)
	var reads atomic.Int64
	inner := newTestServer(browserIssues()...).Handler()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/incidents/inc-003" {
				inner.ServeHTTP(w, r)
				return
			}
			rows := leftoverObjects(12)
			rows[0].(map[string]any)["what"] = fmt.Sprintf("read %d",
				reads.Add(1))
			d["leftovers"] = rows
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(d)
		}))
	t.Cleanup(srv.Close)
	return srv
}

// The same on the issue page: "Show all 12" outlives the redraw of an
// issue that changed.
func TestBrowserLeftoversStayExpandedAcrossRedraw(t *testing.T) {
	srv := changingDetail(t)
	tab := pagingTab(t, srv, 1280, 800, "/issues/inc-003#timeline",
		"[data-level-note]")
	const sec = `'[data-fold="leftovers"]'`
	before := evalString(t, tab, `(() => {
  document.querySelector(".lo-line button").click();
  document.querySelector(".lo-all").click();
  return `+shownRows+`(`+sec+`) + "," +
    document.querySelector(`+sec+` + " .lo-what").textContent;
})()`)
	if !strings.HasPrefix(before, "12|0,read ") {
		t.Fatalf("before the redraw the list shows %q", before)
	}
	refreshPage(t, tab)
	// The update is drawn in place by itself.
	after := evalString(t, tab, `(() => {
  return `+shownRows+`(`+sec+`) + "," +
    document.querySelector(`+sec+` + " .lo-what").textContent;
})()`)
	if !strings.HasPrefix(after, "12|0,read ") {
		t.Errorf("after the redraw the list shows %q, want all 12", after)
	}
	if after == before {
		t.Fatalf("the issue was not redrawn (%q): the test proves nothing",
			after)
	}
}

// The line names the configured age of an old problem, and no number
// when the answer has none.
func TestBrowserOldProblemsLineNamesTheConfiguredDays(t *testing.T) {
	for days, want := range map[int]string{
		90: "3 old problems not shown in the issues (already " +
			"failing for 90+ days when kwatch first saw them)",
		0: "3 old problems not shown in the issues (already " +
			"failing for a long time when kwatch first saw them)",
	} {
		ov := overviewPaging(3)
		op := ov["oldProblems"].(map[string]any)
		delete(op, "days")
		if days > 0 {
			op["days"] = days
		}
		srv := listsServer(t, map[string]any{"/api/overview": ov})
		tab := pagingTab(t, srv, 1280, 800, "/", ".ov-old")
		if got := evalString(t, tab, `document.querySelector(
  ".ov-old-h").textContent`); got != want {
			t.Errorf("days %d: the line is %q, want %q", days, got, want)
		}
	}
}

// twoFailureLeftovers is a leftover group of two objects, one of which
// fails in two ways.
func twoFailureLeftovers() incident.Incident {
	p := leftoverIssue()
	extra := detection.Finding{Entity: inventory.CoreID("secret",
		"policy-system", "policy-tls"), Reason: "SecretUnused",
		Summary: "Nothing uses this Secret", Leftover: p.Root}
	p.Members[extra.Key()] = extra
	return p
}

// The count in the heading, the rows and the cut all count objects: an
// object with two failures is one row.
func TestBrowserLeftoverCountMatchesRows(t *testing.T) {
	served := newTestServer(twoFailureLeftovers())
	srv := httptest.NewServer(served.Handler())
	t.Cleanup(srv.Close)
	tab := pagingTab(t, srv, 1280, 800, "/issues/left", ".lo-line")
	got := evalString(t, tab, `(() => {
  const sec = document.querySelector('[data-fold="leftovers"]');
  return [document.querySelector(".lo-line").textContent,
    sec.querySelector(".sec-h").textContent,
    sec.querySelectorAll(".lo-row").length,
    sec.querySelector(".lo-what").textContent].join("|");
})()`)
	want := "2 objects left behind · See which →|" +
		"Objects left behind (2)|2|Nothing uses this Secret; " +
		"TLS certificate expired on 2025-01-01"
	if got != want {
		t.Errorf("the leftover list says\n%q, want\n%q", got, want)
	}
}

// A cut list says what it left out: objects on the issue page, problems
// on the Overview.
func TestBrowserCutListsNameWhatIsLeftOut(t *testing.T) {
	d := leftoverDetail(t)
	d["leftoverObjects"] = 130
	d["leftovers"] = leftoverObjects(100)
	ov := overviewPaging(50)
	ov["oldProblems"].(map[string]any)["count"] = 60
	srv := listsServer(t, map[string]any{"/api/overview": ov,
		"/api/incidents/inc-003": d})
	const cut = `[...document.querySelectorAll(".ev-cap")].
  map((p) => p.textContent).filter((s) => s.startsWith("+")).join(",")`
	tab := pagingTab(t, srv, 1280, 800, "/issues/inc-003", ".lo-line")
	if got := evalString(t, tab, cut); got !=
		"+30 more objects not shown." {
		t.Errorf("the issue page's cut says %q", got)
	}
	tab = pagingTab(t, srv, 1280, 800, "/", ".ov-old")
	if got := evalString(t, tab, cut); got !=
		"+10 more problems not shown." {
		t.Errorf("the Overview's cut says %q", got)
	}
}

// holdServer is the real server over a reader whose cluster is scaling.
func holdServer(t *testing.T, nonProd bool) *httptest.Server {
	t.Helper()
	served := newTestServer()
	now := time.Now()
	served.SetIncidents(&pagingIncidents{nonProd: nonProd,
		fakeIncidents: fakeIncidents{items: browserIssues()},
		scaling: incident.ScalingState{Held: true,
			Since: now.Add(-5 * time.Minute), Expected: now.Add(time.Hour)}})
	srv := httptest.NewServer(served.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// "Paging held while the cluster scales" is for a cluster that pages:
// a non-production cluster does not show it.
func TestBrowserNoPagingHeldLineOnNonProduction(t *testing.T) {
	const holds = `String(document.querySelectorAll(".ov-hold").length)`
	tab := pagingTab(t, holdServer(t, false), 1280, 800, "/", "#ov-say")
	if got := evalString(t, tab, holds); got != "1" {
		t.Fatalf("a production cluster shows %s hold lines, want 1", got)
	}
	tab = pagingTab(t, holdServer(t, true), 1280, 800, "/", "#ov-say")
	if got := evalString(t, tab, holds); got != "0" {
		t.Errorf("a non-production cluster shows %s hold lines", got)
	}
}

// An old problem's age reads as the issue's "Lowered to Low" line does
// (incident.spokenDays): a year starts at 365.25 days, and one is "a".
func TestBrowserOldProblemAgeWords(t *testing.T) {
	ov := overviewPaging(4)
	items := ov["oldProblems"].(map[string]any)["items"].([]any)
	for i, days := range []int{45, 365, 400, 900} {
		items[i].(map[string]any)["ageDays"] = days
	}
	srv := listsServer(t, map[string]any{"/api/overview": ov})
	tab := pagingTab(t, srv, 1280, 800, "/", ".ov-old")
	got := evalString(t, tab, `[...document.querySelectorAll(
  ".ov-old .lo-age")].map((a) => a.textContent).join("|")`)
	want := "for 45 days|for about 12 months|for about a year|" +
		"for about 2 years"
	if got != want {
		t.Errorf("ages read %q, want %q", got, want)
	}
}
