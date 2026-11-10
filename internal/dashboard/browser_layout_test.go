//go:build browser

package dashboard

import (
	"context"
	"testing"

	"github.com/chromedp/chromedp"
)

// layoutCheck is a page at one size and a script that says what is
// wrong with its layout, or "".
type layoutCheck struct {
	name, path, ready string
	w, h              int64
	script            string
}

// layoutChecks are the layout promises the dashboard makes: a compact
// Issues toolbar with a screenful of rows, a kwatch verdict that never
// says "working" beside a "still loading" notice, one row of links on a
// phone, settings advice only for a refused channel, and the
// notifications switch in the side bar.
var layoutChecks = []layoutCheck{
	{name: "issues toolbar", path: "/issues", w: 1440, h: 900,
		ready: "#issues-table tbody tr", script: `(() => {
  const f = document.querySelector("[data-filterbox]").
    getBoundingClientRect();
  if (f.height > 150) return "toolbar is " + Math.round(f.height) + "px tall";
  const rows = [...document.querySelectorAll("#issues-table tbody tr")].
    filter((r) => r.getBoundingClientRect().bottom <= innerHeight).length;
  if (rows < 12) return "only " + rows + " rows on screen";
  return "";
})()`},
	{name: "kwatch verdict", path: "/kwatch", w: 1440, h: 900,
		ready: ".ov-say", script: `(() => {
  const say = document.querySelector(".ov-say").textContent;
  const loading = [...document.querySelectorAll(".ov-notes .notice")].
    some((n) => /loading/i.test(n.textContent));
  if (loading && /working/.test(say)) return "says " + say + " while loading";
  return "";
})()`},
	{name: "phone nav", path: "/", w: 390, h: 844,
		ready: ".nav a", script: `(() => {
  const tops = new Set([...document.querySelectorAll(".nav a")].
    map((a) => Math.round(a.getBoundingClientRect().top)));
  if (tops.size > 1) return "the links wrap onto " + tops.size + " rows";
  const nav = document.querySelector(".nav");
  const last = [...nav.querySelectorAll("a")].pop();
  const cut = last.getBoundingClientRect().right -
    nav.getBoundingClientRect().right;
  if (cut > 0.5) return "'" + last.textContent.trim() + "' is cut off by " +
    Math.round(cut) + "px";
  const wide = document.documentElement.scrollWidth - innerWidth;
  return wide > 0 ? "the page scrolls sideways by " + wide + "px" : "";
})()`},
	{name: "desktop nav label", path: "/", w: 1440, h: 900,
		ready: ".nav a", script: `(() => {
  const a = document.querySelector('.nav a[data-nav="kwatch"]');
  const pre = a.querySelector(".nav-pre");
  const word = document.createRange();
  word.setStart(pre.firstChild, 0);
  word.setEnd(pre.firstChild, 6);
  const gap = a.querySelector(".nav-end").getBoundingClientRect().left -
    word.getBoundingClientRect().right;
  return gap >= 2 ? "" : "'kwatch' and 'health' touch (" + gap + "px)";
})()`},
	{name: "provider fix advice", path: "/", w: 1440, h: 900,
		ready: ".nav a", script: `(() => {
  const down = KW.providerFix({name: "slack", reason: "provider_unavailable"});
  if (down !== "kwatch keeps trying.") return "an outage says: " + down;
  const no = KW.providerFix({name: "slack", reason: "provider_rejected"});
  if (!/^Fix: update slack/.test(no)) return "a refusal says: " + no;
  return "";
})()`},
	{name: "notifications switch", path: "/", w: 1440, h: 900,
		ready: "[data-ntf-toggle]", script: `(() => {
  const b = document.querySelector("[data-ntf-toggle]");
  return b.getClientRects().length ? "" : "the switch is not shown";
})()`},
}

// checkLayouts runs every layout check in tab.
func checkLayouts(t *testing.T, ctx context.Context, base string) {
	t.Helper()
	for _, c := range layoutChecks {
		err := chromedp.Do(ctx,
			chromedp.EmulateViewport(c.w, c.h),
			chromedp.Navigate(base+c.path),
			chromedp.WaitVisible(chromedp.CSS(c.ready)),
		)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		got, err := chromedp.Run(ctx, chromedp.Evaluate[string](c.script))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		} else if got != "" {
			t.Errorf("%s: %s", c.name, got)
		}
	}
}
