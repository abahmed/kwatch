//go:build browser

package dashboard

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// latchScript: KW.api answers status 0 when reading the body fails
// (the 15 s abort fires mid-body), and a poller whose load throws or
// rejects still runs again when asked. It answers "status|loads".
const latchScript = `(async () => {
  const real = window.fetch;
  window.fetch = () => Promise.resolve({ status: 200,
    headers: { get: () => "" },
    text: () => Promise.reject(new Error("aborted")) });
  const res = await KW.api("/api/overview");
  window.fetch = real;
  let n = 0;
  const load = () => {
    n++;
    if (n === 1) throw new Error("render failed");
    if (n === 2) return Promise.reject(new Error("load failed"));
    return Promise.resolve(60000);
  };
  const wait = () => new Promise((r) => setTimeout(r, 50));
  let p;
  try { p = KW.poller(load, 60000); } catch (e) { return "threw:" + n; }
  await wait(); p.now();
  await wait(); p.now();
  await wait(); p.stop();
  return res.status + "|" + n;
})()`

func TestBrowserLoadFailuresDoNotStopRefreshing(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1")))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](latchScript,
		chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatal(err)
	}
	if want := "0|3"; got != want {
		t.Errorf("api status and loads = %q, want %q", got, want)
	}
}

// signoutScript makes the push unsubscribe and /logout both fail.
const signoutScript = `(() => {
  KW.notify.signOut = () => Promise.reject(new Error("push"));
  const real = window.fetch;
  window.fetch = (u, o) => /logout/.test(u)
    ? Promise.reject(new Error("offline")) : real(u, o);
  document.querySelector("[data-signout]").click();
})()`

// Sign out always ends on the sign-in page, even when stopping pushes
// and the /logout call both fail.
func TestBrowserSignOutAlwaysReachesSignIn(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = chromedp.Run(tab, chromedp.Evaluate[chromedp.Void](
		signoutScript)); err != nil {
		t.Fatal(err)
	}
	_, err = chromedp.Run(tab, chromedp.Poll[bool](
		`location.pathname === "/signin"`))
	if err != nil {
		t.Fatalf("not on /signin after a failed sign-out: %v", err)
	}
}
