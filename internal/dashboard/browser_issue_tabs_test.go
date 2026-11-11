//go:build browser

package dashboard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// tabsState says which tab is open, the address's hash, how many
// question parts the header card still holds, the questions shown in
// the Summary tab, whether Who was alerted? opens the Alerts tab and
// whether What kwatch found is in the Summary tab.
const tabsState = `(() => {
  const card = document.querySelector(".d-sum");
  const sel = document.querySelector("[role=tab][aria-selected=true]");
  const sum = document.querySelector("[data-panel=summary]");
  const rows = [...sum.querySelectorAll("[data-ds-row]")].
    filter((r) => !r.hidden).map((r) => r.dataset.dsRow).join(",");
  return [sel.dataset.tab, location.hash,
    card.querySelectorAll(
      "[data-ds-row], [data-round], .ds-notes, .ds-stage").length,
    rows, !!document.querySelector(
      "[data-panel=alerts] > .ds-alerted [data-ds-row=told]"),
    !!sum.querySelector("[data-round]:not([hidden])")].join("|");
})()`

// tabRoles is "ok" when every tab names its panel and the panel names
// the tab back.
const tabRoles = `(() => {
  const list = document.querySelector("[role=tablist]");
  const tabs = [...list.querySelectorAll("[role=tab]")];
  const bad = tabs.filter((t) => {
    const p = document.getElementById(t.getAttribute("aria-controls"));
    return !p || p.getAttribute("role") !== "tabpanel" ||
      p.getAttribute("aria-labelledby") !== t.id;
  });
  return tabs.map((t) => t.dataset.tab).join(",") +
    (bad.length ? "|bad" : "|ok");
})()`

// The header card holds only the header; the questions are in the
// Summary tab, which opens first; What's wrong? and What should I do?
// always answer; the tab is kept in the address across a reload; the
// arrow keys move along the tabs.
func TestBrowserIssueQuestionsInSummaryTab(t *testing.T) {
	actx, srv := browserSetup(t)
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(srv.URL+"/issues/inc-002"),
		chromedp.WaitVisible(chromedp.CSS(".d-tab")),
		chromedp.WaitReady(chromedp.CSS(".ds-r-why")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, tab, tabsState); got !=
		"summary||0|what,why,next|true|true" {
		t.Errorf("layout: %q, want the questions in Summary only", got)
	}
	if got := evalString(t, tab, tabRoles); got !=
		"summary,timeline,evidence,impact,alerts,details|ok" {
		t.Errorf("tabs: %q", got)
	}
	checkPlainAnswers(t, tab)
	err = chromedp.Do(tab, chromedp.Focus("#tab-summary"),
		chromedp.KeyEvent(kb.ArrowRight))
	if err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, tab, `document.activeElement.id + "|" +
	  location.hash`); got != "tab-timeline|#timeline" {
		t.Errorf("arrow right: %q, want the Timeline tab focused", got)
	}
	err = chromedp.Do(tab, chromedp.Click("#tab-evidence"),
		chromedp.Reload(),
		chromedp.WaitVisible(chromedp.CSS("[data-panel=evidence]")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got := evalString(t, tab, tabsState+` + "|" + document.querySelector(
	  "[data-panel=summary]").hidden`)
	if !strings.HasPrefix(got, "evidence|#evidence|0|") ||
		!strings.HasSuffix(got, "|true") {
		t.Errorf("after a reload: %q, want Proof open from the address", got)
	}
	for _, p := range problems.take() {
		t.Error(p)
	}
}

// checkPlainAnswers: an issue whose sentence only repeats its title
// has no description line, What's wrong? says so, and What should I
// do? gives the read-only commands to start with.
func checkPlainAnswers(t *testing.T, tab context.Context) {
	t.Helper()
	got := evalString(t, tab, `[
	  document.querySelector("[data-ds-lead]").hidden,
	  document.querySelector(".ds-r-what dd").textContent,
	  document.querySelector(".ds-r-next dd").textContent].join("|")`)
	for _, want := range []string{"true|What the title says",
		"kubectl -n web describe deployment/app-inc-002",
		"kubectl -n web logs deployment/app-inc-002"} {
		if !strings.Contains(got, want) {
			t.Errorf("answers lack %q: %q", want, got)
		}
	}
}

// An update in place keeps the tab the reader picked and the address,
// and leaves the questions in their tab.
func TestBrowserIssueTabKeptByUpdate(t *testing.T) {
	srv := changingDetail(t)
	tab := pagingTab(t, srv, 1280, 600, "/issues/inc-003",
		"[data-level-note]")
	err := chromedp.Do(tab, chromedp.Click("#tab-timeline"))
	if err != nil {
		t.Fatal(err)
	}
	before := strings.Split(evalString(t, tab, updateState), "|")
	refreshPage(t, tab)
	after := strings.Split(evalString(t, tab, updateState), "|")
	if after[1] == before[1] {
		t.Fatalf("not redrawn: %v", after)
	}
	got := evalString(t, tab, tabsState)
	if after[0] != "timeline" ||
		!strings.HasPrefix(got, "timeline|#timeline|0|") {
		t.Errorf("after the update: %v %q, want Timeline kept", after, got)
	}
}

// headOrder is the header card's parts in order, whether the whole
// title line shows outside any fold, its text and the meta line.
const headOrder = `(() => {
  const head = document.querySelector(".d-sum > .ds-head");
  const l = document.querySelector("[data-ds-lead]");
  return [[...head.children].filter((n) => !n.hidden &&
      n.getClientRects().length).map((n) => n.tagName.toLowerCase() +
      (n.className ? "." + n.className.split(" ")[0] : "")).join(","),
    !l.hidden && !l.closest("details") && l.getClientRects().length > 0,
    l.textContent, document.querySelector("[data-ds-where]").textContent].
    join("|");
})()`

// The whole title is plain text right under the heading and the meta
// line, never in a fold, and stays so after the page reads again; the
// meta line never says "for for".
func TestBrowserIssueWholeTitleUnderHeading(t *testing.T) {
	d := stubDetail()
	d["fullTitle"] = longNode
	d["title"] = longNode[:120] + "…"
	srv := stubIssueServer(t, d)
	tab := pagingTab(t, srv, 1280, 800, "/issues/stub-1", ".ds-r-why")
	for _, when := range []string{"first", "after a read"} {
		got := strings.Split(evalString(t, tab, headOrder), "|")
		if got[0] != "div.ds-top,h1,ul.ds-where,p.ds-lead" ||
			got[1] != "true" || got[2] != longNode {
			t.Errorf("%s: header %q", when, got)
		}
		if strings.Contains(got[3], "for for") ||
			!strings.Contains(got[3], "for 3 h") {
			t.Errorf("%s: meta line %q", when, got[3])
		}
		refreshPage(t, tab)
	}
}

// On a phone the tab bar stays at the top of the window while a long
// tab scrolls.
func TestBrowserIssueTabBarSticks(t *testing.T) {
	actx, srv := browserSetup(t)
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(srv.URL+"/issues/inc-002"),
		chromedp.WaitVisible(chromedp.CSS(".d-tabbar")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got := evalString(t, tab, `(() => { scrollTo(0, 900);
	  const r = document.querySelector(".d-tabbar").getBoundingClientRect();
	  return Math.round(scrollY > 0 ? r.top : -1) + ""; })()`)
	if got != "0" {
		t.Errorf("tab bar top at %s px after scrolling, want 0", got)
	}
	// A link to a tab past the edge scrolls the strip to it.
	err = chromedp.Do(tab, chromedp.Navigate(srv.URL+"/issues/inc-002#alerts"),
		chromedp.WaitVisible(chromedp.CSS("[data-panel=alerts]")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got = evalString(t, tab, `(() => {
	  const r = document.querySelector("#tab-alerts").getBoundingClientRect();
	  return (r.left >= 0 && r.right <= innerWidth) + ""; })()`)
	if got != "true" {
		t.Error("the open Alerts tab is off the edge of the strip")
	}
}
