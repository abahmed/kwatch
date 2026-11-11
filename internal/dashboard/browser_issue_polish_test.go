//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// polishPage opens url at a window size and waits for a node.
func polishPage(
	t *testing.T, srvURL, path, wait string, w int64,
) (func(string) string, func()) {
	t.Helper()
	actx, _ := browserSetup(t)
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srvURL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(w, 900),
		chromedp.Navigate(srvURL+path),
		chromedp.WaitVisible(chromedp.CSS(wait)),
		chromedp.Sleep(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	text := func(js string) string {
		got, err := chromedp.Run(tab, chromedp.Evaluate[string](js))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	return text, func() {
		for _, p := range problems.take() {
			t.Error(p)
		}
	}
}

// The heading is the short title, and on a phone it takes three lines
// at most; the whole title is the description line under it, never cut,
// with no "Full title" fold.
func TestBrowserIssueHeadingIsShort(t *testing.T) {
	srv := stubIssueServer(t, stubDetail())
	text, done := polishPage(t, srv.URL, "/issues/stub-1",
		".ds-r-why", 390)
	defer done()
	got := text(`(() => { const h = document.querySelector("h1");
	  const lines = h.getBoundingClientRect().height /
	    parseFloat(getComputedStyle(h).lineHeight);
	  return h.textContent.length + "|" + Math.round(lines); })()`)
	parts := strings.Split(got, "|")
	if parts[0] == "" || len(stubTitle) <= atoiOr(parts[0]) ||
		atoiOr(parts[0]) > 80 {
		t.Errorf("heading is not short: %q", got)
	}
	if atoiOr(parts[1]) > 3 {
		t.Errorf("heading takes %s lines on a phone", parts[1])
	}
	if got := text(`(() => {
	  const l = document.querySelector("[data-ds-lead]");
	  const cut = l.scrollWidth > l.clientWidth + 1;
	  return (document.querySelector("[data-full-title]") ? "fold|" : "") +
	    (l.hidden ? "hidden|" : "") + (cut ? "cut|" : "") + l.textContent;
	})()`); got != stubTitle {
		t.Errorf("the description line is %q, want the whole title", got)
	}
}

func atoiOr(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// Copy summary carries the proof line, who was alerted and when, and
// the cluster name when the page knows it.
func TestBrowserCopySummaryIsTicketReady(t *testing.T) {
	srv := stubIssueServer(t, stubDetail())
	text, done := polishPage(t, srv.URL, "/issues/stub-1",
		".ds-r-why", 1280)
	defer done()
	grab := `(() => { window.__t = ""; KW.copy = (b, s) => { window.__t = s; };
	  document.querySelector("[data-copy-summary]").click();
	  return window.__t; })()`
	plain := text(grab)
	for _, want := range []string{
		"Proof: System.InvalidOperationException: Unable to resolve",
		"Alerted: Slack at ",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("summary lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Cluster:") {
		t.Errorf("summary names a cluster the page doesn't know:\n%s", plain)
	}
	text(`(() => { KW.cluster = "demo-cluster"; return ""; })()`)
	withName := text(grab)
	if !strings.Contains(withName, "Cluster: demo-cluster") {
		t.Errorf("summary lacks the cluster name:\n%s", withName)
	}
}

// On a phone the proof line and the log lines wrap by default; the
// toggle still switches it off. On a desktop nothing wraps at first.
func TestBrowserPhoneWrapsProofAndLogs(t *testing.T) {
	d := stubDetail()
	long := strings.Repeat("connection refused to db.example.com:5432 ", 6)
	d["capturedLogLines"] = []string{long}
	srv := stubIssueServer(t, d)
	probe := `(() => { const b = document.querySelector("[data-logs]");
	  const r = document.querySelector(".ds-raw-t");
	  const w = document.querySelector("[data-wrap-logs]");
	  const rows = [...b.children];
	  return [b.classList.contains("wrap"), w.getAttribute("aria-pressed"),
	    getComputedStyle(r).whiteSpace,
	    rows.every((x) => x.scrollWidth <= x.clientWidth + 1)].join("|");
	})()`
	text, done := polishPage(t, srv.URL, "/issues/stub-1#evidence",
		"[data-logs] div", 390)
	defer done()
	if got := text(probe); got != "true|true|pre-wrap|true" {
		t.Errorf("phone does not wrap by default: %q", got)
	}
	if got := text(`(() => { document.querySelector(
	  "[data-wrap-logs]").click(); return document.querySelector(
	  "[data-logs]").classList.contains("wrap") + ""; })()`); got != "false" {
		t.Errorf("Wrap lines cannot switch it off: %q", got)
	}
	text2, done2 := polishPage(t, srv.URL, "/issues/stub-1#evidence",
		"[data-logs] div", 1280)
	defer done2()
	if got := text2(probe); !strings.HasPrefix(got, "false|false|nowrap") {
		t.Errorf("desktop wraps at first: %q", got)
	}
}

// With no alert sent the Alerts tab asks "Why this level", not "Why
// you got this alert".
func TestBrowserAlertsWhyThisLevel(t *testing.T) {
	reason := "Lowered to Low: only minor problems are left; Low sends no alert."
	quiet := stubDetail()
	delete(quiet, "timing")
	quiet["why"] = map[string]any{"alertLevelReason": reason}
	told := stubDetail()
	told["why"] = map[string]any{"alertLevelReason": reason}
	for name, c := range map[string]struct {
		d    map[string]any
		want string
	}{"none": {quiet, "Why this level: "},
		"sent": {told, "Why you got this alert: "}} {
		srv := stubIssueServer(t, c.d)
		text, done := polishPage(t, srv.URL, "/issues/stub-1#alerts",
			"[data-why-level]", 1280)
		got := text(`document.querySelector("[data-why-level]").textContent`)
		if !strings.HasPrefix(got, c.want+reason[:10]) {
			t.Errorf("%s: %q, want it to start %q", name, got, c.want)
		}
		done()
	}
}

// Who was alerted?, at the top of the Alerts tab, shows the
// acknowledge command, to copy, never a button that acts; once
// acknowledged it is gone.
func TestBrowserAlertsTabShowsAckCommand(t *testing.T) {
	srv := stubIssueServer(t, stubDetail())
	text, done := polishPage(t, srv.URL, "/issues/stub-1#alerts",
		"[data-panel=alerts] > .ds-alerted .ds-r-told", 1280)
	defer done()
	got := text(`(() => { const r = document.querySelector(".ds-r-told");
	  const c = r.querySelector("code, .cmd");
	  return r.textContent.includes("Not acknowledged yet") + "|" +
	    (c ? c.textContent : "") + "|" +
	    r.querySelectorAll("button").length; })()`)
	if !strings.HasPrefix(got, "true|kubectl -n shop annotate deployment/api ") {
		t.Errorf("top card has no acknowledge command: %q", got)
	}
	d := stubDetail()
	d["ack"] = map[string]any{"by": "sam", "on": "Deployment/shop/api",
		"since": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}
	srv2 := stubIssueServer(t, d)
	text2, done2 := polishPage(t, srv2.URL, "/issues/stub-1#alerts",
		".ds-r-told", 1280)
	defer done2()
	got = text2(`document.querySelector(".ds-r-told").textContent`)
	if strings.Contains(got, "kubectl") {
		t.Errorf("an acknowledged issue still shows the command: %q", got)
	}
}

// "Show every time" drops the date of today's stamps and never breaks
// inside a stamp.
func TestBrowserTimelineStampsAreShort(t *testing.T) {
	srv := stubIssueServer(t, stubDetail())
	text, done := polishPage(t, srv.URL, "/issues/stub-1#timeline",
		"[data-timeline] .tl-times", 390)
	defer done()
	got := text(`(() => { const s = [...document.querySelectorAll(
	  ".tl-times .tl-stamp")];
	  return s.length + "|" + s.every((x) => /^(yesterday )?\d\d:\d\d$/.
	    test(x.textContent) && getComputedStyle(x).whiteSpace === "nowrap") +
	    "|" + s.map((x) => x.textContent).join(","); })()`)
	if !strings.Contains(got, "|true|") || strings.HasPrefix(got, "0|") {
		t.Errorf("stamps are not short and unbroken: %q", got)
	}
}

// At 390px the tab strip either fits or says clearly that it scrolls.
func TestBrowserPhoneTabsFitOrHint(t *testing.T) {
	srv := stubIssueServer(t, stubDetail())
	text, done := polishPage(t, srv.URL, "/issues/stub-1",
		".d-tabs", 390)
	defer done()
	got := text(`(() => { const bar = document.querySelector(".d-tabbar");
	  const tabs = [...document.querySelectorAll(".d-tab")];
	  const fits = tabs[tabs.length - 1].getBoundingClientRect().right <=
	    innerWidth;
	  return fits + "|" + bar.classList.contains("more") + "|" +
	    getComputedStyle(bar, "::after").content; })()`)
	switch {
	case strings.HasPrefix(got, "true|false"):
	case strings.HasPrefix(got, "false|true") &&
		!strings.HasSuffix(got, "|none") && !strings.HasSuffix(got, "|normal"):
	default:
		t.Errorf("tabs run off the edge without a hint: %q", got)
	}
}

// The help panel names things as the page does.
func TestBrowserHelpAgreesWithTerms(t *testing.T) {
	srv := stubIssueServer(t, stubDetail())
	text, done := polishPage(t, srv.URL, "/issues/stub-1",
		".ds-r-why", 1280)
	defer done()
	got := text(`(() => { const h = document.querySelector(".legend-pop").
	  textContent; return h.includes('"' + KW.TERMS.ask.todo + '"') + "|" +
	  h.includes(KW.TERMS.severity.notify.level.toLowerCase()) + "|" +
	  h.includes("sent a message"); })()`)
	if got != "true|true|false" {
		t.Errorf("help text disagrees with the page: %q", got)
	}
}
