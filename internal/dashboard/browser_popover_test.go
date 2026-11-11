//go:build browser

package dashboard

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// popovers are the Issues page's popovers: what opens one, and the
// popover. The column menu opens from a header cell.
var popovers = []struct{ name, open, pop string }{
	{"view options", "[data-vopts] > summary", ".vopts-pop"},
	{"columns", "[data-cols-btn]", "[data-cols-pop]"},
	{"search tips", "[data-shint] > summary", ".shint-pop"},
	{"column menu", ".itable th .it-menu", ".colmenu"},
}

// popoverScript scrolls the table so its heading is stuck, opens a
// popover and says what covers it, or "". Every point inside it must
// hit the popover itself, and it stays inside the window.
const popoverScript = `(async (open, sel) => {
  const frame = () => new Promise((r) => requestAnimationFrame(() => r()));
  const box = KW.scrollBox();
  if (box) box.scrollTop = 400;
  await frame();
  // View options and Columns wait behind "Filters".
  if (!document.querySelector(open).getClientRects().length) {
    document.querySelector(".ftoggle").click();
    await frame();
  }
  document.querySelector(open).click();
  await frame(); await frame();
  const pop = [...document.querySelectorAll(sel)].
    find((p) => p.getClientRects().length);
  if (!pop) return "it did not open";
  const r = pop.getBoundingClientRect();
  if (r.left < 0 || r.right > innerWidth) {
    return "it reaches past the window: " + Math.round(r.left) + ".." +
      Math.round(r.right) + " of " + innerWidth;
  }
  // A grid of points over the whole popover, 20px apart.
  const at = [];
  for (let y = r.top + 6; y < r.bottom - 4; y += 20) {
    for (let x = r.left + 6; x < r.right - 4; x += 20) at.push([x, y]);
  }
  let out = "";
  for (const [x, y] of at) {
    const el = document.elementFromPoint(x, y);
    if (!out && (!el || !pop.contains(el))) {
      out = "at " + Math.round(x) + "," + Math.round(y) + " the " +
        (el ? el.tagName.toLowerCase() + "." + el.className : "nothing") +
        " is on top";
    }
  }
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape",
    bubbles: true }));
  document.body.click();
  return out;
})`

// Popovers sit above the table's stuck heading and the toolbar.
func TestBrowserPopoversAboveStickyHeading(t *testing.T) {
	tab, base := issuesTab(t)
	for _, p := range popovers {
		if err := chromedp.Do(tab, chromedp.Navigate(base+"/issues"),
			chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")),
			chromedp.Sleep(300*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		got, err := chromedp.Run(tab, chromedp.Evaluate[string](
			fmt.Sprintf("%s(%q, %q)", popoverScript, p.open, p.pop),
			chromedp.EvalAwaitPromise))
		if err != nil {
			t.Errorf("%s: %v", p.name, err)
		} else if got != "" {
			t.Errorf("%s: %s", p.name, got)
		}
	}
}

// issuesTab is a signed-in tab at a desktop size and the server.
func issuesTab(t *testing.T) (context.Context, string) {
	t.Helper()
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	if err := chromedp.Do(tab,
		chromedp.EmulateViewport(1440, 900)); err != nil {
		t.Fatal(err)
	}
	return tab, srv.URL
}

// contrastScript gives the contrast of el's text against what is drawn
// behind it (its own background, else the first ancestor's).
const contrastScript = `((sel) => {
  const el = document.querySelector(sel);
  const rgb = (c) => {
    const m = c.match(/[\d.]+/g).map(Number);
    return { v: m.slice(0, 3), a: m.length > 3 ? m[3] : 1 };
  };
  let bg = null;
  for (let n = el; n && !bg; n = n.parentElement) {
    const c = rgb(getComputedStyle(n).backgroundColor);
    if (c.a > 0.5) bg = c.v;
  }
  bg = bg || [255, 255, 255];
  const lum = (v) => {
    const l = v.map((x) => { x /= 255;
      return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4); });
    return 0.2126 * l[0] + 0.7152 * l[1] + 0.0722 * l[2];
  };
  const a = lum(rgb(getComputedStyle(el).color).v), b = lum(bg);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
})`

// hoverButtons are buttons whose hovered text must stay readable;
// open: hover it while its popover is open (filled).
var hoverButtons = []struct {
	sel  string
	open bool
}{
	{"[data-vopts] > summary", false}, {"[data-vopts] > summary", true},
	{"[data-cols-btn]", false}, {"[data-cols-btn]", true},
	{".stsw-b", false}, {".stsw-b[aria-pressed=true]", false},
	{".nav a", false}, {".fchip", false},
}

// A hovered button keeps its text readable (4.5:1), in the light and
// the dark theme, and with high contrast on.
func TestBrowserHoverKeepsContrast(t *testing.T) {
	tab, base := issuesTab(t)
	for _, mode := range []struct{ theme, contrast string }{
		{"light", ""}, {"dark", ""}, {"light", "more"}, {"dark", "more"}} {
		_, err := chromedp.Call(tab, emulation.SetEmulatedMedia,
			emulation.SetEmulatedMediaParams{
				Features: []*emulation.MediaFeature{{
					Name: "prefers-color-scheme", Value: mode.theme}}})
		if err != nil {
			t.Fatal(err)
		}
		err = chromedp.Do(tab, chromedp.Navigate(base+"/issues"),
			chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")))
		if err != nil {
			t.Fatal(err)
		}
		_, err = chromedp.Run(tab, chromedp.Evaluate[bool](`((c) => {
  if (c) document.documentElement.dataset.contrast = c; return true;
})("`+mode.contrast+`")`))
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range hoverButtons {
			ratio := hoverContrast(t, tab, b.sel, b.open)
			if ratio < 4.5 {
				t.Errorf("%s %s %s (open %t) hovered: contrast %.2f",
					mode.theme, mode.contrast, b.sel, b.open, ratio)
			}
		}
	}
}

// hoverContrast puts the mouse on sel and measures its text contrast.
func hoverContrast(
	t *testing.T, tab context.Context, sel string, open bool,
) float64 {
	t.Helper()
	pt, err := chromedp.Run(tab, chromedp.Evaluate[[]float64](`((s, o) => {
  const b = document.querySelector(s);
  if (!b.getClientRects().length) document.querySelector(".ftoggle").click();
  const d = b.closest("details");
  if (d) d.open = o; else if (o) b.click();
  if (!o) document.body.click();
  const r = b.getBoundingClientRect();
  return [r.left + r.width / 2, r.top + r.height / 2];
})(`+fmt.Sprintf("%q, %t", sel, open)+`)`))
	if err != nil || len(pt) != 2 {
		t.Fatalf("%s: %v", sel, err)
	}
	err = chromedp.Do(tab,
		chromedp.MouseEvent(input.DispatchMouseEventTypeMouseMoved, pt[0], pt[1]),
		chromedp.Sleep(250*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	r, err := chromedp.Run(tab, chromedp.Evaluate[float64](
		fmt.Sprintf("%s(%q)", contrastScript, sel)))
	if err != nil {
		t.Fatalf("%s: %v", sel, err)
	}
	return r
}
