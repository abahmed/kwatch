//go:build browser

package dashboard

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// a11yScript lists what a page gets wrong, in words: an image without
// a text alternative, a button or link without a name, a form field
// without a name or label, a repeated id, or no language. Only what is
// shown to assistive technology counts (a hidden field stands behind a
// custom control; an aria-hidden one only feeds password managers).
const a11yScript = `(() => {
  const out = [];
  const shown = (el) => el.getClientRects().length > 0 &&
    !el.closest("[aria-hidden=true]");
  const named = (el) => (el.textContent || "").trim() ||
    el.getAttribute("aria-label") || el.getAttribute("title") ||
    el.getAttribute("aria-labelledby");
  const what = (el) => el.outerHTML.slice(0, 120);
  if (!document.documentElement.lang) out.push("html has no lang");
  document.querySelectorAll("img").forEach((el) => {
    if (shown(el) && !el.hasAttribute("alt")) out.push("img alt: " + what(el));
  });
  document.querySelectorAll("svg[role=img]").forEach((el) => {
    if (shown(el) && !el.getAttribute("aria-label") &&
        !el.getAttribute("aria-labelledby") && !el.querySelector("title")) {
      out.push("svg label: " + what(el));
    }
  });
  document.querySelectorAll("button, a[href]").forEach((el) => {
    if (shown(el) && !named(el)) out.push("no name: " + what(el));
  });
  document.querySelectorAll("input, select, textarea").forEach((el) => {
    if (el.type === "hidden" || !shown(el)) return;
    if (!el.name && !el.id) out.push("no name or id: " + what(el));
    const label = el.closest("label") || (el.id &&
      document.querySelector('label[for="' + CSS.escape(el.id) + '"]')) ||
      el.getAttribute("aria-label") || el.getAttribute("aria-labelledby");
    if (!label) out.push("no label: " + what(el));
  });
  const seen = {};
  document.querySelectorAll("[id]").forEach((el) => {
    if (seen[el.id]) out.push("repeated id: " + el.id);
    seen[el.id] = true;
  });
  return out;
})()`

// overflowScript is how far the page scrolls sideways, in pixels.
const overflowScript = `document.documentElement.scrollWidth - innerWidth`

// checkPage loads url at w×h and reports what the browser and the
// in-page checks found.
func checkPage(
	t *testing.T, ctx context.Context, problems *pageProblems,
	url string, w, h int64, name string,
) {
	t.Helper()
	err := chromedp.Do(ctx,
		chromedp.EmulateViewport(w, h),
		chromedp.Navigate(url),
		chromedp.WaitVisible(chromedp.CSS("h1")),
		// The page reads its data after its head is drawn.
		chromedp.Sleep(time.Second),
	)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return
	}
	a11y, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](a11yScript))
	if err != nil {
		t.Errorf("%s: a11y: %v", name, err)
	}
	overflow, err := chromedp.Run(ctx,
		chromedp.Evaluate[float64](overflowScript))
	if err != nil {
		t.Errorf("%s: overflow: %v", name, err)
	}
	for _, p := range problems.take() {
		t.Errorf("%s: %s", name, p)
	}
	for _, p := range a11y {
		t.Errorf("%s: %s", name, p)
	}
	if overflow > 1 {
		t.Errorf("%s: scrolls sideways by %.0fpx", name, overflow)
	}
}

// filtersScript scrolls the Issues table's own frame to its bottom
// and says where the filters ended up.
const filtersScript = `(() => {
  const box = KW.scrollBox();
  if (!box) return "the table has no scroll frame";
  box.scrollTop = box.scrollHeight;
  const f = document.querySelector("[data-filterbox]").
    getBoundingClientRect();
  if (window.scrollY !== 0) return "the page scrolled: " + window.scrollY;
  if (f.top < 0 || f.bottom > innerHeight) {
    return "filters out of view: " + f.top + ".." + f.bottom;
  }
  return "";
})()`

// checkIssuesFiltersStay: on a wide screen, scrolling the table keeps
// the filters on screen.
func checkIssuesFiltersStay(t *testing.T, ctx context.Context, base string) {
	t.Helper()
	err := chromedp.Do(ctx,
		chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(base+"/issues"),
		chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")),
	)
	if err != nil {
		t.Fatalf("issues scroll: %v", err)
	}
	got, err := chromedp.Run(ctx, chromedp.Evaluate[string](filtersScript))
	if err != nil {
		t.Fatalf("issues scroll: %v", err)
	}
	if got != "" {
		t.Errorf("issues scroll: %s", got)
	}
}
