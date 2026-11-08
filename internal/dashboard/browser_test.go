//go:build browser

package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/abahmed/kwatch/internal/incident"
)

// The dashboard in a real (headless) browser: every page at a desktop
// and a phone size loads without console errors, exceptions or CSP
// violations, draws its heading, never scrolls sideways and passes
// basic accessibility checks. Run with `make verify-dashboard-browser`;
// without Chrome or Chromium the test skips.

// browserSizes are the window sizes every page is checked at.
var browserSizes = []struct{ w, h int64 }{{1280, 800}, {390, 844}}

// browserIssues are enough issues for the Issues table to scroll.
func browserIssues() []incident.Incident {
	var out []incident.Incident
	for i := 0; i < 40; i++ {
		it := testIncident(fmt.Sprintf("inc-%03d", i),
			[]string{"shop", "pay", "web"}[i%3],
			-time.Duration(i)*time.Minute)
		if i%4 == 0 {
			it.Tier = incident.Page
		}
		if i == 2 {
			it.Cause = browserRoundCause()
		}
		out = append(out, it)
	}
	return out
}

func TestBrowserDashboardPages(t *testing.T) {
	ctx, srv := browserSetup(t)
	pages := []string{"/", "/issues", "/issues/inc-001", "/activity",
		"/kwatch", "/map", "/map?view=failing"}
	for _, size := range browserSizes {
		tab, problems := browserTab(t, ctx)
		signInBrowser(t, tab, srv.URL)
		for _, p := range pages {
			name := fmt.Sprintf("%s@%dx%d", p, size.w, size.h)
			checkPage(t, tab, problems, srv.URL+p, size.w, size.h, name)
		}
		if size.w > 900 {
			checkIssuesFiltersStay(t, tab, srv.URL)
		}
	}
}

// The sign-in page is checked signed out: its API calls are refused,
// which the page handles.
func TestBrowserSignInPage(t *testing.T) {
	ctx, srv := browserSetup(t)
	for _, size := range browserSizes {
		tab, problems := browserTab(t, ctx)
		name := fmt.Sprintf("/signin@%dx%d", size.w, size.h)
		checkPage(t, tab, problems, srv.URL+"/signin", size.w, size.h,
			name)
	}
}

// browserSetup starts the dashboard and a browser, or skips.
func browserSetup(t *testing.T) (context.Context, *httptest.Server) {
	t.Helper()
	path := findChrome()
	if path == "" {
		t.Skip("no Chrome or Chromium found (set CHROME_PATH)")
	}
	served := newTestServer(browserIssues()...)
	served.SetModel(browserMapModel(t))
	served.sources.releases.Store(browserReleases())
	srv := httptest.NewServer(served.Handler())
	t.Cleanup(srv.Close)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(path))
	if os.Getenv("CI") != "" {
		// CI runners may not allow Chrome's sandbox; the page is our own.
		opts = append(opts, chromedp.NoSandbox)
	}
	actx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(cancel)
	return actx, srv
}

// findChrome is the browser to run: CHROME_PATH, else a known name.
func findChrome() string {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable",
		"chromium", "chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// pageProblems collects what the browser reports while a page runs.
type pageProblems struct {
	mu   sync.Mutex
	list []string
}

func (p *pageProblems) add(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = append(p.list, s)
}

func (p *pageProblems) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.list
	p.list = nil
	return out
}

// browserTab opens a tab that reports console errors, uncaught
// exceptions and error entries of the browser log (CSP violations and
// files that failed to load among them).
func browserTab(
	t *testing.T, actx context.Context,
) (context.Context, *pageProblems) {
	t.Helper()
	ctx, cancel := chromedp.NewContext(actx)
	t.Cleanup(cancel)
	ctx, stop := context.WithTimeout(ctx, 3*time.Minute)
	t.Cleanup(stop)
	problems := &pageProblems{}
	messages := chromedp.Console(ctx)
	go func() {
		for m, err := range messages {
			if err != nil {
				return
			}
			problems.note(m)
		}
	}()
	return ctx, problems
}

func (p *pageProblems) note(m chromedp.ConsoleMessage) {
	switch {
	case m.Type == chromedp.ConsoleException:
		p.add("exception: " + m.Text)
	case m.Type != chromedp.ConsoleError || apiStatus(m):
	case m.Source != "":
		p.add(m.Source + ": " + m.Text + " " + m.URL)
	default:
		p.add("console error: " + m.Text)
	}
}

// apiStatus is a refused or failed API answer: the page shows those
// itself (sign-in, error panels), so they are not page errors. A
// missing script, stylesheet or image is.
func apiStatus(m chromedp.ConsoleMessage) bool {
	if m.Source != "network" {
		return false
	}
	u, err := url.Parse(m.URL)
	return err == nil && strings.HasPrefix(u.Path, "/api/")
}

// signInBrowser gives the tab a session cookie from a real sign-in.
func signInBrowser(t *testing.T, ctx context.Context, base string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/login",
		strings.NewReader(`{"password":"`+testPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", base)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var value string
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			value = c.Value
		}
	}
	if value == "" {
		t.Fatalf("sign in: %d, no session cookie", res.StatusCode)
	}
	httpOnly := true
	_, err = chromedp.Call(ctx, network.SetCookie, network.SetCookieParams{
		Name: sessionCookie, Value: value, URL: base, HTTPOnly: &httpOnly,
		SameSite: network.CookieSameSiteStrict})
	if err != nil {
		t.Fatal(err)
	}
}
