//go:build browser

package dashboard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

// alertsOn grants notifications to the page, turns browser alerts on
// as the sidebar switch saves it, and opens the Overview again so
// notify.js reads that.
func alertsOn(t *testing.T, tab context.Context, base string) {
	t.Helper()
	if _, err := chromedp.CallBrowser(tab, browser.SetPermission,
		browser.SetPermissionParams{Origin: base,
			Permission: &browser.PermissionDescriptor{
				Name: "notifications"},
			Setting: browser.PermissionSettingGranted}); err != nil {
		t.Fatal(err)
	}
	err := chromedp.Do(tab, chromedp.Navigate(base+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1")))
	if err != nil {
		t.Fatal(err)
	}
	evalAwait(t, tab, `Promise.resolve(String(localStorage.setItem(
	  "kwatch.notify", JSON.stringify({ enabled: true }))))`)
	err = chromedp.Do(tab, chromedp.Navigate(base+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1")))
	if err != nil {
		t.Fatal(err)
	}
}

// readsScript restarts the watcher with its 30 s interval caught, and
// answers the list reads of: the start, an interval tick while the
// page is in view with the live stream up, and a tick while hidden.
const readsScript = `(async () => {
  const wait = () => new Promise((r) => setTimeout(r, 150));
  const realApi = KW.api, realEvery = window.setInterval;
  const realLive = KW.live.connected;
  let calls = 0, tick = null;
  KW.notify.stop();
  await wait();
  KW.api = (path) => {
    if (/^\/api\/incidents\?/.test(path)) calls++;
    return Promise.resolve({ status: 200, etag: "",
      data: { incidents: [], totalPages: 1 } });
  };
  KW.live.connected = () => true;
  window.setInterval = (fn) => { tick = fn; return realEvery(() => {}, 1e9); };
  KW.notify.start(document.body);
  window.setInterval = realEvery;
  await wait();
  const out = [calls];
  calls = 0;
  if (tick) tick();
  await wait();
  out.push(calls);
  calls = 0;
  Object.defineProperty(document, "hidden",
    { configurable: true, get: () => true });
  if (tick) tick();
  await wait();
  out.push(calls);
  delete document.hidden;
  KW.api = realApi;
  KW.live.connected = realLive;
  return out.join("|");
})()`

// The browser-alerts check reads nothing while alerts are off; with
// them on it reads both lists at start, leaves a tick in view to the
// live stream, and keeps reading while the tab is hidden.
func TestBrowserNotifyReadsOnlyWhenAlertsAreOn(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1"))); err != nil {
		t.Fatal(err)
	}
	if got := evalAwait(t, tab, readsScript); got != "0|0|0" {
		t.Errorf("alerts off: reads %q, want none", got)
	}
	alertsOn(t, tab, srv.URL)
	if got := evalAwait(t, tab, readsScript); got != "2|0|2" {
		t.Errorf("alerts on: reads %q, want 2|0|2", got)
	}
}

// evalAwait runs script and waits for the promise it returns.
func evalAwait(t *testing.T, tab context.Context, script string) string {
	t.Helper()
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](script,
		chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// The Issues tab title has no count of its own (the badge and the
// favicon have it), so "(n)" in front only ever means changes not
// seen yet, and a prefix with separators never stacks.
func TestBrowserIssuesTitleHasNoCount(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	for status, label := range map[string]string{"active": "Active",
		"resolved": "Resolved", "all": "All"} {
		err := chromedp.Do(tab, chromedp.Navigate(srv.URL+
			"/issues?status="+status))
		if err == nil {
			_, err = chromedp.Run(tab, chromedp.Poll[bool](
				`document.title.includes("`+label+` issues")`,
				chromedp.WithPollingTimeout(10*time.Second)))
		}
		if err != nil {
			t.Fatal(status, err)
		}
		got := evalAwait(t, tab, `Promise.resolve(document.title)`)
		if got == "" || got[0] == '(' {
			t.Errorf("%s: title %q", status, got)
		}
	}
	got := evalAwait(t, tab, `(async () => {
	  KW.unseen = 1234;
	  KW.setTitle("(5) (1,234) Issues - kwatch");
	  KW.setTitle(KW.baseTitle);
	  const t = document.title;
	  KW.unseen = 0;
	  KW.setTitle(KW.baseTitle);
	  return t + "|" + document.title;
	})()`)
	if !strings.HasPrefix(got, "(1,234) Issues - ") ||
		strings.Count(got, "(") != 1 ||
		!strings.Contains(got, "|Issues - ") {
		t.Errorf("titles %q", got)
	}
}
