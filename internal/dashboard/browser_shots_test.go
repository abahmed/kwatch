//go:build browser

package dashboard

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// shotPages are the pages TestBrowserShots saves.
var shotPages = []struct{ name, path, ready string }{
	{"overview", "/", ".ov-say"},
	{"issues", "/issues", "#issues-table tbody tr"},
	{"issue", "/issues/inc-002", ".d-tab"},
	{"issue-alerts", "/issues/inc-002#alerts", ".ds-r-told"},
	{"activity", "/activity", ".act-row"},
	{"kwatch", "/kwatch", ".ov-say"},
}

// TestBrowserShots saves a screenshot of each main page, the window
// and the whole page, at a desktop and a phone size into KW_SHOTS, to
// look at a change by eye. Without KW_SHOTS it skips.
func TestBrowserShots(t *testing.T) {
	dir := os.Getenv("KW_SHOTS")
	if dir == "" {
		t.Skip("set KW_SHOTS to a folder to save screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, srv := browserSetup(t)
	for _, size := range []struct{ w, h int64 }{{1440, 900}, {390, 844}} {
		tab, _ := browserTab(t, ctx)
		signInBrowser(t, tab, srv.URL)
		for _, p := range shotPages {
			name := fmt.Sprintf("%s-%d", p.name, size.w)
			err := chromedp.Do(tab, chromedp.EmulateViewport(size.w, size.h),
				chromedp.Navigate(srv.URL+p.path),
				chromedp.WaitVisible(chromedp.CSS(p.ready)),
				settleFonts(), chromedp.Sleep(time.Second))
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			save(t, tab, filepath.Join(dir, name+".png"), false)
			save(t, tab, filepath.Join(dir, name+"-full.png"), true)
		}
		if size.w > 900 {
			popShot(t, tab, srv.URL, filepath.Join(dir, "popover-hover.png"))
		}
	}
	longTitleShots(t, ctx, dir)
	kwatchShots(t, ctx, dir)
}

// kwatchShots saves the kwatch page with what kwatch watches known
// (some kinds it may not read), at both sizes.
func kwatchShots(t *testing.T, ctx context.Context, dir string) {
	t.Helper()
	served := newTestServer(browserIssues()...)
	served.SetCoverage(func() kube.Coverage {
		return kube.Coverage{
			Namespaces: kube.NamespaceCoverage{All: true, Watched: 34},
			Kinds: kube.KindCoverage{Watched: 40, Synced: 37,
				Reasons: map[string]int{kube.ReasonPermissionDenied: 3}},
			SyncedAt: testEpoch,
		}
	})
	srv := httptest.NewServer(served.Handler())
	t.Cleanup(srv.Close)
	for _, w := range []int64{1440, 390} {
		tab, _ := browserTab(t, ctx)
		signInBrowser(t, tab, srv.URL)
		err := chromedp.Do(tab, chromedp.EmulateViewport(w, 900),
			chromedp.Navigate(srv.URL+"/kwatch"),
			chromedp.WaitVisible(chromedp.CSS(".ov-say")),
			settleFonts(), chromedp.Sleep(500*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		save(t, tab, filepath.Join(dir,
			fmt.Sprintf("kwatch-cov-%d-full.png", w)), true)
	}
}

// longNode is the whole title of an issue whose names are long.
const longNode = "Node ip-10-0-12-34.region-1.compute.internal is " +
	"failing; two possible causes: it has lost its network or pod " +
	"web-7c9d8f6b5-x2k4q in shop cannot reach its database through " +
	"the cluster DNS service"

// longTitleShots saves the issue page of an issue with a long title
// and long names, and of a full issue (release, proof, steps), at both
// sizes.
func longTitleShots(t *testing.T, ctx context.Context, dir string) {
	t.Helper()
	d := stubDetail()
	d["fullTitle"] = longNode
	d["title"] = longNode[:120] + "…"
	srv := stubIssueServer(t, d)
	for _, w := range []int64{1440, 390} {
		tab, _ := browserTab(t, ctx)
		signInBrowser(t, tab, srv.URL)
		err := chromedp.Do(tab, chromedp.EmulateViewport(w, 900),
			chromedp.Navigate(srv.URL+"/issues/stub-1"),
			chromedp.WaitVisible(chromedp.CSS(".ds-r-why")),
			settleFonts(), chromedp.Sleep(500*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		save(t, tab, filepath.Join(dir, fmt.Sprintf("issue-long-%d.png", w)),
			false)
	}
	full := stubIssueServer(t, stubDetail())
	for _, w := range []int64{1440, 390} {
		tab, _ := browserTab(t, ctx)
		signInBrowser(t, tab, full.URL)
		err := chromedp.Do(tab, chromedp.EmulateViewport(w, 900),
			chromedp.Navigate(full.URL+"/issues/stub-1"),
			chromedp.WaitVisible(chromedp.CSS(".ds-r-why")),
			settleFonts(), chromedp.Sleep(500*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		save(t, tab, filepath.Join(dir, fmt.Sprintf("issue-full-%d.png", w)),
			true)
	}
}

// popShot saves the Issues page with the table scrolled, View options
// open and the mouse on its button.
func popShot(t *testing.T, tab context.Context, base, path string) {
	t.Helper()
	err := chromedp.Do(tab, chromedp.Navigate(base+"/issues"),
		chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")),
		settleFonts())
	if err != nil {
		t.Fatal(err)
	}
	pt, err := chromedp.Run(tab, chromedp.Evaluate[[]float64](`(() => {
  document.querySelector(".ftoggle").click();
  KW.scrollBox().scrollTop = 400;
  const s = document.querySelector("[data-vopts] > summary");
  s.click();
  const r = s.getBoundingClientRect();
  return [r.left + r.width / 2, r.top + r.height / 2]; })()`))
	if err != nil || len(pt) != 2 {
		t.Fatalf("popover: %v", err)
	}
	err = chromedp.Do(tab,
		chromedp.MouseEvent(input.DispatchMouseEventTypeMouseMoved,
			pt[0], pt[1]), chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	save(t, tab, path, false)
}

// save writes a screenshot of the window, or of the whole page.
func save(t *testing.T, tab context.Context, path string, full bool) {
	t.Helper()
	shot := chromedp.CaptureScreenshot()
	if full {
		shot = chromedp.FullScreenshot(100)
	}
	png, err := chromedp.Run(tab, shot)
	if err == nil {
		err = os.WriteFile(path, png, 0o644)
	}
	if err != nil {
		t.Errorf("%s: %v", path, err)
	}
}
