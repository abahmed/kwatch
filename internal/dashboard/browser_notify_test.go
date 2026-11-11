//go:build browser

package dashboard

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// notifyScript runs an open tab's notification rules (notify.js)
// through an issue that settles and is then confirmed, and one that
// settles quietly, on a list of one page and on a list of more. It
// answers what each check would tell, joined by "|".
const notifyScript = `(() => {
  const L = KW.notifyLogic;
  const prefs = { min: "digest", resolved: true };
  const old = new Date(Date.now() - 3600e3).toISOString();
  const it = (id, sev, state, untold) => ({ id: id, severity: sev,
    state: state, untold: untold, title: id + " crash-loops",
    namespace: "shop", workload: id, firstSeen: old });
  const run = (pages) => {
    const snap = (active, resolved, at) => L.snapshot(
      { incidents: active, totalPages: pages }, { incidents: resolved }, at);
    const a = it("a", "page", "open");
    const t0 = Date.now() - 90e3;
    const checks = [
      snap([a], [], t0),
      snap([a, it("s", "notify", "settling", true),
        it("q", "page", "settling", true)], [], t0 + 30e3),
      snap([a, it("s", "notify", "open"),
        it("q", "page", "settling", true)], [], t0 + 60e3),
      snap([a, it("s", "notify", "open")],
        [it("q", "page", "resolved", true)], t0 + 90e3)];
    const out = [];
    for (let i = 1; i < checks.length; i++) {
      out.push(L.changes(checks[i - 1], checks[i], prefs).map(
        (c) => c.item.id + ":" + c.what).join(" "));
    }
    return out.join("|");
  };
  return run(1) + " / " + run(2);
})()`

// The open tab's notifications follow the push rules: an issue is new
// once it is confirmed, never while it settles, and one that settles
// quietly is never told, even on a list cut to one page.
func TestBrowserNotifyTellsOnlyConfirmedIssues(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1")))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](notifyScript))
	if err != nil {
		t.Fatal(err)
	}
	if want := "|s:new| / |s:new|"; got != want {
		t.Errorf("told %q, want %q", got, want)
	}
}
