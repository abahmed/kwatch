//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Settings is one item in the side bar that opens its own page; the
// side bar shows none of the controls, and the page keeps them.
func TestBrowserSettingsPage(t *testing.T) {
	srv := listsServer(t, nil)
	for _, w := range []int64{1440, 390} {
		text, done := polishPage(t, srv.URL, "/settings", ".settings", w)
		got := text(`(() => {
		  const vis = (s) => [...document.querySelectorAll(s)]
		    .some((e) => e.getClientRects().length);
		  const side = document.querySelector(".side");
		  return [
		    document.querySelector('[data-nav="settings"]')
		      .getAttribute("aria-current"),
		    vis(".settings [data-theme-b]"), vis(".settings [data-tz]"),
		    vis(".settings [data-ntf-toggle]"),
		    !!side.querySelector("[data-theme-b], [data-ntf-toggle]"),
		    document.title].join("|");
		})()`)
		if !strings.HasPrefix(got, "page|true|true|true|false|Settings - ") {
			t.Errorf("%dpx: %q", w, got)
		}
		done()
	}
}

// The check that told two causes apart is said once on the Summary tab
// (in Why?), without a second "kwatch checked:" prefix.
func TestBrowserCheckFactSaidOnce(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(srv.URL+"/issues/inc-002"),
		chromedp.WaitVisible(chromedp.CSS(".rd")),
		chromedp.Sleep(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got := evalString(t, tab, `document.querySelector("#main").innerText`)
	if n := strings.Count(got, "DNS resolves db fine"); n != 1 {
		t.Errorf("fact said %d times in %q", n, got)
	}
	if strings.Contains(got, "Check: kwatch checked") {
		t.Errorf("duplicate prefix in %q", got)
	}
}
