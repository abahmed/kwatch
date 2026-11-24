//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// uxActivity is an /api/activity answer: one issue started ten minutes
// ago and one resolved twenty minutes ago.
func uxActivity() map[string]any {
	return map[string]any{"events": []any{
		map[string]any{"id": "inc-001", "at": listAgo(10 * time.Minute),
			"kind": "new", "severity": "notify", "workload": "api",
			"title": "api in shop is not ready", "namespace": "shop"},
		map[string]any{"id": "inc-002", "at": listAgo(20 * time.Minute),
			"kind": "resolved", "severity": "page", "workload": "web",
			"title": "web in shop is not ready", "namespace": "shop"}},
		"total": 2, "kinds": map[string]int{"new": 1, "resolved": 1},
		"untold": 0}
}

// Activity opens on the last 24 hours, offers 7 days, keeps the last
// choice in this browser, says how long ago each change was and no
// longer repeats the title under the sentence.
func TestBrowserActivityWindowsAndTimes(t *testing.T) {
	srv := listsServer(t, map[string]any{"/api/activity": uxActivity()})
	tab := pagingTab(t, srv, 390, 844, "/activity", ".act-row")
	pressed := `document.querySelector('[data-act-since][aria-pressed="true"]')
	  .dataset.actSince`
	got := evalString(t, tab, pressed+` + "|" + [...document.querySelectorAll(
	  "[data-act-since]")].map((b) => b.textContent).join(",") + "|" +
	  document.querySelectorAll(".act-title").length + "|" +
	  document.querySelector(".act-ago").textContent`)
	if got != "24h|Last hour,Last 24 hours,Last 7 days|0|10 min ago" {
		t.Errorf("activity: %q", got)
	}
	err := chromedp.Do(tab, chromedp.Click(`[data-act-since="7d"]`),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Navigate(srv.URL+"/activity"),
		chromedp.WaitVisible(chromedp.CSS(".act-row")))
	if err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, tab, pressed+` + "|" + location.search`); got !=
		"7d|?since=7d" {
		t.Errorf("the last window is not kept: %q", got)
	}
}

// With times in UTC a short time says so.
func TestBrowserShortTimesSayUTC(t *testing.T) {
	srv := listsServer(t, map[string]any{"/api/activity": uxActivity()})
	tab := pagingTab(t, srv, 1280, 800, "/activity", ".act-row")
	err := chromedp.Do(tab, chromedp.Click(`[data-nav="settings"]`),
		chromedp.WaitVisible(chromedp.CSS(`[data-tz="utc"]`)),
		chromedp.Click(`[data-tz="utc"]`),
		chromedp.Click(`[data-nav="activity"]`),
		chromedp.WaitVisible(chromedp.CSS(".act-row")),
		chromedp.Sleep(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got := evalString(t, tab,
		`document.querySelector(".act-at time").textContent`)
	if !strings.HasSuffix(got, " UTC") {
		t.Errorf("a UTC time without UTC: %q", got)
	}
}

// The Overview says what changed since this browser's last visit.
func TestBrowserOverviewSinceLastVisit(t *testing.T) {
	srv := listsServer(t, map[string]any{"/api/activity": uxActivity()})
	tab := pagingTab(t, srv, 1280, 800, "/", ".ov-say")
	err := chromedp.Do(tab, chromedp.Evaluate[chromedp.Void](`localStorage.setItem(
	  "kwatch.visits", JSON.stringify({ base: Date.now() - 7200000,
	  visit: Date.now() - 3600000, items: {} }))`),
		chromedp.Reload(), chromedp.WaitVisible(chromedp.CSS(".ov-since")))
	if err != nil {
		t.Fatal(err)
	}
	got := evalString(t, tab, `document.querySelector(".ov-since").textContent`)
	if !strings.HasPrefix(got, "Since your last visit (") ||
		!strings.Contains(got, "): 1 new · 1 resolved. See what changed") {
		t.Errorf("since strip: %q", got)
	}
}

// Settings, on every page: the theme, text size and high contrast;
// View options no longer has them, nor a second rows-per-page.
func TestBrowserSettingsThemeAndText(t *testing.T) {
	srv := listsServer(t, nil)
	for _, w := range []int64{1280, 390} {
		tab := pagingTab(t, srv, w, 800, "/settings", ".settings")
		err := chromedp.Do(tab, chromedp.Click(`[data-theme-b="dark"]`),
			chromedp.SetValue("[data-look-text]", "l"),
			chromedp.Evaluate[chromedp.Void](`document.querySelector("[data-look-text]")
			  .dispatchEvent(new Event("change"))`),
			chromedp.Sleep(300*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		got := evalString(t, tab, `[document.documentElement.dataset.theme,
		  document.documentElement.dataset.text,
		  JSON.parse(localStorage.getItem("kwatch.view")).theme,
		  !!document.querySelector("[data-vo-text], [data-vo-size], " +
		    "[data-vo-contrast]"),
		  getComputedStyle(document.body).backgroundColor].join("|")`)
		if got != "dark|l|dark|false|rgb(21, 23, 27)" {
			t.Errorf("%dpx settings: %q", w, got)
		}
		err = chromedp.Do(tab, chromedp.Click(`[data-theme-b="system"]`),
			chromedp.SetValue("[data-look-text]", "m"),
			chromedp.Evaluate[chromedp.Void](`document.querySelector("[data-look-text]")
			  .dispatchEvent(new Event("change"))`))
		if err != nil {
			t.Fatal(err)
		}
		if got := evalString(t, tab, `String(document.documentElement.
		  getAttribute("data-theme"))`); got != "null" {
			t.Errorf("%dpx System left %q", w, got)
		}
	}
}

// The kwatch health page has a setup checklist with a how-to for each
// missing item, and says "This cluster has no name" once.
func TestBrowserHealthSetupChecklist(t *testing.T) {
	srv := listsServer(t, nil)
	tab := pagingTab(t, srv, 1280, 900, "/kwatch", "#setup")
	got := evalString(t, tab, `[...document.querySelectorAll("#setup li")]
	  .map((li) => li.dataset.check + ":" + li.className).join(",") + "|" +
	  document.querySelector("#setup").textContent + "|" +
	  document.querySelector("[data-no-name]").textContent`)
	for _, want := range []string{"Cluster name:is-todo",
		"Heartbeat:", "At least one alert channel:is-done",
		"Secrets watched:", "Permissions complete:",
		"Set app.clusterName in the chart's values"} {
		if !strings.Contains(got, want) {
			t.Errorf("checklist lacks %q: %q", want, got)
		}
	}
	if strings.Count(got, "This cluster has no name") != 1 {
		t.Errorf("the no-name notice repeats itself: %q", got)
	}
}

// An address the dashboard has no page for says so, inside the page.
func TestBrowserUnknownPathIsPageNotFound(t *testing.T) {
	srv := listsServer(t, nil)
	tab := pagingTab(t, srv, 1280, 800, "/", ".ov-say")
	err := chromedp.Do(tab,
		chromedp.Evaluate[chromedp.Void](`KW.go("/no-such-page")`),
		chromedp.WaitVisible(chromedp.CSS(".lead-404")))
	if err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, tab, `document.querySelector("#main h1")
	  .textContent`); got != "Page not found" {
		t.Errorf("unknown page: %q", got)
	}
}

// On an iPhone outside the Home Screen app, Browser alerts says how to
// get them rather than "can't".
func TestBrowserIPhoneAlertsSayAddToHomeScreen(t *testing.T) {
	srv := listsServer(t, nil)
	actx, _ := browserSetup(t)
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	ua := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) " +
		"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 " +
		"Mobile/15E148 Safari/604.1"
	_, err := chromedp.Call(tab, emulation.SetUserAgentOverride,
		emulation.SetUserAgentOverrideParams{UserAgent: ua})
	if err != nil {
		t.Fatal(err)
	}
	_, err = chromedp.Call(tab, page.AddScriptToEvaluateOnNewDocument,
		page.AddScriptToEvaluateOnNewDocumentParams{
			Source: "delete window.Notification;"})
	if err != nil {
		t.Fatal(err)
	}
	err = chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(srv.URL+"/settings"),
		chromedp.WaitVisible(chromedp.CSS(".settings")),
		chromedp.Click("[data-ntf-toggle]"),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got := evalString(t, tab,
		`document.querySelector("[data-ntf-hint]").textContent`)
	if got != "On iPhone: Share → Add to Home Screen, open kwatch from "+
		"there, then turn on alerts." {
		t.Errorf("iPhone hint: %q", got)
	}
}
