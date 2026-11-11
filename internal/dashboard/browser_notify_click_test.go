//go:build browser

package dashboard

import (
	"testing"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// swClickScript runs the dashboard's real /sw.js against a stand-in
// worker scope (its clients are fakes that log what they are asked),
// clicks a notification for the page `url` with these open tabs, and
// answers the log, one call per line.
const swClickScript = `(async (url, tabs, focusFails) => {
  const src = await (await fetch("/sw.js")).text();
  const on = {}, log = [];
  const open = tabs.map((u) => ({ url: location.origin + u,
    focused: false, visibilityState: "visible",
    focus() {
      log.push("focus " + u);
      return focusFails ? Promise.reject(new Error("no")) :
        Promise.resolve(this);
    },
    postMessage(m) { log.push("message " + m.kwatch + " " + m.url); } }));
  const scope = { addEventListener: (n, f) => { on[n] = f; },
    skipWaiting() {}, navigator: {},
    registration: { scope: location.origin + "/" },
    clients: { claim() {}, matchAll: async () => open,
      openWindow: async (u) => { log.push("open " + u); } } };
  new Function("self", src)(scope);
  let done = null;
  on.notificationclick({ waitUntil: (p) => { done = p; },
    notification: { data: { url }, close() { log.push("close"); } } });
  await done;
  return log.join("\n");
})`

// A click on a notification closes it and brings kwatch forward on the
// notification's page: a new tab when none is open, the open kwatch tab
// (which moves there itself) otherwise, the tab already on that page
// as it is, and a new tab when the open one can't be focused. A link
// that leaves the dashboard opens the Overview instead.
func TestBrowserNotificationClickOpensKwatch(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	// The script below evaluates sw.js, which the page's CSP forbids.
	if _, err := chromedp.Call(tab, page.SetBypassCSP,
		page.SetBypassCSPParams{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1"))); err != nil {
		t.Fatal(err)
	}
	o := srv.URL
	cases := []struct {
		name, url, tabs string
		fails           bool
		want            string
	}{
		{"no tab", "/issues/inc-001", "[]", false,
			"close\nopen " + o + "/issues/inc-001"},
		{"a tab elsewhere", "/issues/inc-001", `["/activity"]`, false,
			"close\nfocus /activity\nmessage open /issues/inc-001"},
		{"a tab on it", "/issues/inc-001",
			`["/activity", "/issues/inc-001"]`, false,
			"close\nfocus /issues/inc-001"},
		{"the test one", "/", `["/issues"]`, false,
			"close\nfocus /issues\nmessage open /"},
		{"focus refused", "/", `["/issues"]`, true,
			"close\nfocus /issues\nopen " + o + "/"},
		{"another site", "//example.com/x", "[]", false,
			"close\nopen " + o + "/"},
	}
	for _, c := range cases {
		script := swClickScript + `("` + c.url + `", ` + c.tabs + `, ` +
			map[bool]string{true: "true", false: "false"}[c.fails] + `)`
		got, err := chromedp.Run(tab, chromedp.Evaluate[string](script,
			chromedp.EvalAwaitPromise))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		} else if got != c.want {
			t.Errorf("%s:\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}
