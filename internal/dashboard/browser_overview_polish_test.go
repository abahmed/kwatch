//go:build browser

package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
)

// weekScript draws the Overview's "This week" part from a fixture
// (which cards exist is chosen by `causes` and `trouble`) and answers
// "gridWidth|cardWidth,cardWidth,..|chartWidth|chartCardContentWidth|top",
// where top is how many distinct row tops the main cards have.
func weekScript(causes, trouble bool) string {
	return fmt.Sprintf(`(async () => {
  const days = [];
  for (let i = 0; i < 7; i++) {
    days.push({ date: "2026-11-0" + (i + 1), opened: i + 2,
      resolved: i, paged: i %% 2 });
  }
  const ins = { days, causes: [], nodes: [],
    namespaces: { tiles: [], more: 0 } };
  if (%t) ins.causes = [{ label: "OOMKilled", mode: "oom",
    count: 4, active: 1 }];
  if (%t) ins.namespaces.tiles = [{ namespace: "shop", count: 3,
    urgent: 1, attention: 2 }];
  const box = document.createElement("section");
  box.className = "ov-week";
  box.style.maxWidth = "90rem";
  document.body.prepend(box);
  const tr = { previousWeek: 1, noisiest: [], days: [], namespaces: [] };
  KW.insightsPart(box).set(ins, tr);
  const frame = () => new Promise((r) => requestAnimationFrame(() => r()));
  await frame(); await frame(); await frame();
  const grids = box.querySelectorAll(":scope > .pgrid");
  const main = grids[1];
  const cards = [...main.children].
    map((c) => Math.round(c.getBoundingClientRect().width));
  const tops = new Set([...main.children].
    map((c) => Math.round(c.getBoundingClientRect().top)));
  const chart = box.querySelector(".ip-chart svg");
  const card = box.querySelector(".ip-chart").closest(".pcard");
  const cs = getComputedStyle(card);
  const inner = card.getBoundingClientRect().width -
    parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight) - 2;
  return [Math.round(main.getBoundingClientRect().width), cards.join(","),
    Math.round(chart.getBoundingClientRect().width), Math.round(inner),
    tops.size].join("|");
})()`, causes, trouble)
}

func runWeek(t *testing.T, w, h int64, causes, trouble bool,
	shot string) []int {
	t.Helper()
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	if err := chromedp.Do(tab, chromedp.EmulateViewport(w, h),
		chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1"))); err != nil {
		t.Fatal(err)
	}
	script := weekScript(causes, trouble)
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](script,
		chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("KW_SHOTS"); dir != "" && shot != "" {
		if png, err := chromedp.Run(tab, chromedp.FullScreenshot(90)); err == nil {
			_ = os.WriteFile(filepath.Join(dir, shot+".png"), png, 0o644)
		}
	}
	var grid, c1, c2, chart, inner, tops int
	n, _ := fmt.Sscanf(got, "%d|%d,%d|%d|%d|%d",
		&grid, &c1, &c2, &chart, &inner, &tops)
	if n < 5 {
		_, _ = fmt.Sscanf(got, "%d|%d|%d|%d|%d",
			&grid, &c1, &chart, &inner, &tops)
		c2 = 0
	}
	t.Logf("%s", got)
	return []int{grid, c1, c2, chart, inner, tops}
}

func near(a, b int) bool { d := a - b; return d >= -3 && d <= 3 }

// A lone "Where the trouble is" card fills the row; two cards share it.
func TestBrowserOverviewCardsFillTheRow(t *testing.T) {
	for _, size := range browserSizes {
		name := fmt.Sprintf("%dx%d", size.w, size.h)
		t.Run("lone "+name, func(t *testing.T) {
			r := runWeek(t, size.w, size.h, false, true, "lone-"+name)
			if !near(r[1], r[0]) {
				t.Errorf("lone card is %dpx in a %dpx grid", r[1], r[0])
			}
		})
		t.Run("pair "+name, func(t *testing.T) {
			r := runWeek(t, size.w, size.h, true, true, "pair-"+name)
			if size.w > 700 {
				if !near(r[1]*2+16, r[0]) || !near(r[1], r[2]) {
					t.Errorf("cards are %dpx and %dpx in a %dpx grid", r[1], r[2], r[0])
				}
			} else if !near(r[1], r[0]) || !near(r[2], r[0]) || r[5] != 2 {
				t.Errorf("phone cards are %dpx and %dpx in a %dpx grid, %d rows",
					r[1], r[2], r[0], r[5])
			}
		})
	}
}

// The week chart spreads over its card instead of a fixed width.
func TestBrowserWeekChartFillsItsCard(t *testing.T) {
	for _, size := range browserSizes {
		name := fmt.Sprintf("%dx%d", size.w, size.h)
		t.Run(name, func(t *testing.T) {
			r := runWeek(t, size.w, size.h, true, true, "chart-"+name)
			if !near(r[3], r[4]) {
				t.Errorf("chart is %dpx wide in a card with %dpx of room", r[3], r[4])
			}
		})
	}
}
