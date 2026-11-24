//go:build browser

package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// moreShots are the parts a change may touch beyond the main pages:
// the issue page's other tabs, the Resolved list (empty here), and a
// small issue's map. Each is saved at both sizes, in the dark theme.
var moreShots = []struct{ name, path, ready string }{
	{"issue-timeline", "/issues/inc-002#timeline", "[data-timeline] li"},
	{"issue-proof", "/issues/inc-002#evidence", ".d-tab"},
	{"issue-impact", "/issues/inc-001#impact", ".d-tab"},
	{"issue-details", "/issues/inc-002#details", ".d-tab"},
	{"resolved-empty", "/issues?status=resolved", "[data-empty] *"},
	{"not-found", "/issues/no-such-issue", ".lead-404"},
}

// TestBrowserShotsMore saves, into KW_SHOTS: the sign-in page, the
// issue tabs, the Resolved list, the drawer, the Settings opened on a
// phone, the light theme picked in Settings, and the error state of a
// page whose API fails. Without KW_SHOTS it skips.
func TestBrowserShotsMore(t *testing.T) {
	dir := os.Getenv("KW_SHOTS")
	if dir == "" {
		t.Skip("set KW_SHOTS to a folder to save screenshots")
	}
	ctx, srv := browserSetup(t)
	for _, w := range []int64{1440, 390} {
		tab, _ := browserTab(t, ctx)
		shotTheme(t, tab, "dark")
		step(t, tab, dir, fmt.Sprintf("signin-%d", w), w,
			chromedp.Navigate(srv.URL+"/signin"),
			chromedp.WaitVisible(chromedp.CSS("#pw")))
		signInBrowser(t, tab, srv.URL)
		for _, p := range moreShots {
			step(t, tab, dir, fmt.Sprintf("%s-%d", p.name, w), w,
				chromedp.Navigate(srv.URL+p.path),
				chromedp.WaitVisible(chromedp.CSS(p.ready)))
		}
		step(t, tab, dir, fmt.Sprintf("drawer-%d", w), w,
			chromedp.Navigate(srv.URL+"/issues"),
			chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")),
			chromedp.Click("#issues-table tbody tr td.c-sev"),
			chromedp.WaitVisible(chromedp.CSS(".drw-body .ds-r-why")))
		step(t, tab, dir, fmt.Sprintf("settings-%d", w), w,
			chromedp.Navigate(srv.URL+"/settings"),
			chromedp.WaitVisible(chromedp.CSS(".settings")))
		step(t, tab, dir, fmt.Sprintf("overview-picked-light-%d", w), w,
			chromedp.Navigate(srv.URL+"/settings"),
			chromedp.WaitVisible(chromedp.CSS(".settings")),
			chromedp.Click(`[data-theme-b="light"]`))
		_, _ = chromedp.Run(tab, chromedp.Evaluate[chromedp.Void](
			`localStorage.removeItem("kwatch.view")`))
	}
	errorShots(t, ctx, dir)
}

// errorShots saves the Activity page while its API answers 500.
func errorShots(t *testing.T, ctx context.Context, dir string) {
	t.Helper()
	inner := newTestServer(browserIssues()...).Handler()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/activity" {
				writeError(w, http.StatusInternalServerError, "boom")
				return
			}
			inner.ServeHTTP(w, r)
		}))
	t.Cleanup(srv.Close)
	for _, w := range []int64{1440, 390} {
		tab, _ := browserTab(t, ctx)
		shotTheme(t, tab, "dark")
		signInBrowser(t, tab, srv.URL)
		step(t, tab, dir, fmt.Sprintf("error-%d", w), w,
			chromedp.Navigate(srv.URL+"/activity"),
			chromedp.WaitVisible(chromedp.CSS(".panel")))
	}
}

// step runs the actions at width w and saves the window as name.
func step(t *testing.T, tab context.Context, dir, name string, w int64,
	actions ...chromedp.Action[chromedp.Void]) {
	t.Helper()
	all := append([]chromedp.Action[chromedp.Void]{
		chromedp.EmulateViewport(w, 900)}, actions...)
	all = append(all, settleFonts(), chromedp.Sleep(700*time.Millisecond))
	if err := chromedp.Do(tab, all...); err != nil {
		t.Errorf("%s: %v", name, err)
		return
	}
	save(t, tab, filepath.Join(dir, name+".png"), false)
}
