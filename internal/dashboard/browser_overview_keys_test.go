//go:build browser

package dashboard

import (
	"fmt"
	"strings"
	"testing"
)

// keysProbe answers, per card, "title=key classes=bar classes" and,
// for Where the trouble is, every row's value.
const keysProbe = `(() => [...document.querySelectorAll(".ov-week .pcard")].
  filter((c) => /Top causes|Where the trouble/.test(c.textContent)).
  map((c) => {
    const cls = (sel) => [...new Set([...c.querySelectorAll(sel)].
      map((e) => [...e.classList].filter((x) => /^s-/.test(x)).join("")))].
      sort().join(",");
    return c.querySelector(".pcard-h").textContent + "=" +
      cls(".viz-legend .viz-sw") + "=" + cls(".viz-seg") + "=" +
      [...c.querySelectorAll(".viz-val")].map((v) => v.textContent).
        join(";");
  }).join("\n"))()`

// Top causes and Where the trouble is each carry a key, a colour means
// the same in both (levels for levels, the Opened grey for openings),
// and every namespace shown says how many issues opened in it this
// week, not only the top five of the trends.
func TestBrowserOverviewCardsKeyTheirColours(t *testing.T) {
	ov := overviewAnswer(1, 0, 0, false)
	ins := ov["insights"].(map[string]any)
	ins["causes"] = []any{map[string]any{"label": "Out of memory",
		"mode": "OOMKilled", "count": 4, "active": 1}}
	var tiles []any
	for i := 0; i < 5; i++ {
		tiles = append(tiles, map[string]any{
			"namespace": fmt.Sprintf("ns-%d", i), "severity": "notify",
			"count": 2, "urgent": 1, "attention": 1, "week": i})
	}
	ins["namespaces"] = map[string]any{"tiles": tiles, "more": 0}
	srv := listsServer(t, map[string]any{"/api/overview": ov})
	tab := pagingTab(t, srv, 1280, 900, "/", ".ov-week .viz-legend")
	got := strings.Split(evalString(t, tab, keysProbe), "\n")
	if len(got) != 2 {
		t.Fatalf("cards: %q", got)
	}
	causes := strings.Split(got[0], "=")
	if causes[1] != "s-open" || causes[2] != "s-open" {
		t.Errorf("Top causes keys %q, bars %q", causes[1], causes[2])
	}
	where := strings.Split(got[1], "=")
	if where[1] != "s-low,s-notify,s-page" ||
		strings.Contains(where[2], "s-open") {
		t.Errorf("Where the trouble is keys %q, bars %q", where[1],
			where[2])
	}
	rows := strings.Split(where[3], ";")
	for i, r := range rows {
		if !strings.Contains(r, fmt.Sprintf("%d opened this week", i)) {
			t.Errorf("row %d says %q", i, r)
		}
	}
}
