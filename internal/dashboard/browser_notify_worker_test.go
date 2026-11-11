//go:build browser

package dashboard

import (
	"testing"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

// swKeptScript waits for the page's own start-up work, then answers
// what the browser has: the dashboard's worker registered and in
// control of the page, and the notification a click is meant for
// shown through it with its page, one fact per line.
const swKeptScript = `(async () => {
  await new Promise((r) => setTimeout(r, 1500));
  const list = await navigator.serviceWorker.getRegistrations();
  const out = ["registrations " + list.length];
  const c = navigator.serviceWorker.controller;
  out.push("controller " + (c ? new URL(c.scriptURL).pathname : "none"));
  const reg = await navigator.serviceWorker.getRegistration("/");
  if (!reg) return out.join("\n");
  await reg.showNotification("kwatch notifications work",
    { tag: "kwatch-test", data: { url: "/" } });
  const shown = await reg.getNotifications({ tag: "kwatch-test" });
  out.push("shown " + shown.map((n) => n.data.url).join(","));
  return out.join("\n");
})()`

// The service worker that shows notifications and opens kwatch on a
// click stays registered when the dashboard is opened again. A worker
// that is unregistered while its notification is on screen leaves a
// notification whose click nothing handles, and its push subscription
// ends with it.
func TestBrowserServiceWorkerSurvivesReload(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	if _, err := chromedp.CallBrowser(tab, browser.SetPermission,
		browser.SetPermissionParams{Origin: srv.URL,
			Permission: &browser.PermissionDescriptor{
				Name: "notifications"},
			Setting: browser.PermissionSettingGranted}); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1"))); err != nil {
		t.Fatal(err)
	}
	register := `navigator.serviceWorker.register("/sw.js",
		{ scope: "/" }).then(() => navigator.serviceWorker.ready)
		.then(() => true)`
	if _, err := chromedp.Run(tab, chromedp.Evaluate[bool](register,
		chromedp.EvalAwaitPromise)); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/issues"),
		chromedp.WaitVisible(chromedp.CSS("h1"))); err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](swKeptScript,
		chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatal(err)
	}
	want := "registrations 1\ncontroller /sw.js\nshown /"
	if got != want {
		t.Errorf("after opening the dashboard again:\n%s\nwant\n%s",
			got, want)
	}
}
