//go:build browser

package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// overviewFull is an /api/overview answer with every part of the page:
// bad releases, the week, causes, namespaces, issues that kept coming
// back and busy nodes.
func overviewFull() map[string]any {
	ov := overviewAnswer(2, 3, 1, true)
	ins := ov["insights"].(map[string]any)
	ins["causes"] = []any{
		map[string]any{"label": "Out of memory", "mode": "OOMKilled",
			"count": 9, "active": 2},
		map[string]any{"label": "Image pull failures",
			"mode": "ImagePull", "count": 4, "active": 1},
		map[string]any{"label": "Crash loop", "mode": "CrashLoop",
			"count": 2, "active": 0}}
	var tiles []any
	for i, ns := range []string{"shop", "pay", "web"} {
		tiles = append(tiles, map[string]any{"namespace": ns,
			"severity": "notify", "count": 3 - i, "urgent": 1,
			"attention": 2 - i, "week": 6 - 2*i})
	}
	ins["namespaces"] = map[string]any{"tiles": tiles, "more": 0}
	ins["nodes"] = []any{
		map[string]any{"name": "node-a", "issues": 4, "urgent": 1},
		map[string]any{"name": "node-b", "issues": 2, "urgent": 0}}
	tr := ov["trends"].(map[string]any)
	tr["previousWeek"], tr["previousWeekAlerted"] = 30, 12
	tr["resolveAlerted"] = map[string]any{"count": 9,
		"medianSeconds": 1500}
	tr["noisiest"] = []any{
		map[string]any{"id": "inc-004", "title": "api in shop keeps " +
			"restarting", "namespace": "shop", "workload": "api",
			"kind": "Deployment", "opened": 5, "state": "flapping"},
		map[string]any{"id": "inc-005", "title": "worker in pay is " +
			"not ready", "namespace": "pay", "workload": "worker",
			"kind": "Deployment", "opened": 3, "state": "open"}}
	return ov
}

// rowsProbe answers one line per row of cards that are side by side
// whose tops or bottoms differ by more than 1px, and how many
// <details> the page has.
const rowsProbe = `(() => {
  const bad = [];
  const groups = [...document.querySelectorAll(
    "#main .pgrid, #main .ov-stats, #main .ov-pair")];
  for (const g of groups) {
    const kids = [...g.children].filter((c) => !c.hidden &&
      c.getBoundingClientRect().height > 0);
    const rows = {};
    for (const c of kids) {
      const r = c.getBoundingClientRect();
      (rows[Math.round(r.top)] ||= []).push(r);
    }
    for (const [top, list] of Object.entries(rows)) {
      const b = list.map((r) => r.bottom);
      if (Math.max(...b) - Math.min(...b) > 1) {
        bad.push(g.className + " @" + top + ": " + b.join(","));
      }
    }
  }
  return bad.join("\n") + "|" + document.querySelectorAll(
    "#main details").length + "|" + Math.round(
    document.documentElement.scrollWidth - innerWidth);
})()`

// On a wide screen the Overview's cards that share a row share their
// top and bottom edges; nothing on the page is folded away; and no
// width scrolls sideways. With KW_SHOTS set it saves the page at each
// width (KW_SHOT_TAG names the set: before, after).
func TestBrowserOverviewCardsShareTheirRow(t *testing.T) {
	srv := listsServer(t, map[string]any{"/api/overview": overviewFull()})
	for _, w := range []int64{1440, 2000, 390} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			tab := pagingTab(t, srv, w, 900, "/", ".ov-week .pcard")
			time.Sleep(300 * time.Millisecond)
			got := strings.Split(evalString(t, tab, rowsProbe), "|")
			if w == 1440 && got[0] != "" {
				t.Errorf("ragged rows:\n%s", got[0])
			}
			if got[1] != "0" {
				t.Errorf("%s folds on the Overview", got[1])
			}
			if got[2] != "0" {
				t.Errorf("scrolls sideways by %spx", got[2])
			}
			if dir := os.Getenv("KW_SHOTS"); dir != "" {
				tag := os.Getenv("KW_SHOT_TAG")
				save(t, tab, filepath.Join(dir,
					fmt.Sprintf("overview-%s-%d.png", tag, w)), true)
			}
		})
	}
}
