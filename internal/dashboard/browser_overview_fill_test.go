//go:build browser

package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fillAnswer is the full Overview with n active issues and bad
// releases that make the right card tall (none when rels is 0).
func fillAnswer(n, rels int) map[string]any {
	ov := overviewFull()
	var look []any
	for i := 0; i < n; i++ {
		look = append(look, map[string]any{"id": "inc-" + strconv.Itoa(i),
			"title": "api" + strconv.Itoa(i) + " in shop can't run: " +
				"its image is not built for this node's CPU",
			"namespace": "shop", "kind": "deployment",
			"name": "api" + strconv.Itoa(i), "severity": "notify",
			"state": "open", "since": listAgo(time.Duration(i+1) * time.Hour)})
	}
	ov["lookFirst"] = look
	if counts := ov["counts"].(map[string]any); n > 6 {
		// The counts are 2 urgent, 3 attention and 1 low: keep their
		// total at least the rows, as the server always does.
		counts["attention"] = n - 3
	}
	r := ov["releases"].(map[string]any)
	if rels == 0 {
		r["wrong"], r["bad"] = []any{}, 0
		ov["insights"].(map[string]any)["afterChange"] = nil
		return ov
	}
	var wrong []any
	for i := 0; i < rels; i++ {
		wrong = append(wrong, map[string]any{"name": "svc" + strconv.Itoa(i),
			"namespace": "shop", "kind": "deployment",
			"at": listAgo(time.Duration(i+1) * time.Hour), "verdict": "down"})
	}
	r["wrong"], r["bad"] = wrong, rels
	return ov
}

// fillProbe answers: shown rows, whether the end line shows, the empty
// space under the last shown row inside the issues card, and the
// bottom edges of the two cards.
const fillProbe = `(() => {
  const card = document.querySelector(".ov-pair > .ov-main > .ov-act");
  const rows = [...card.querySelectorAll(".ov-look > li")]
    .filter((l) => !l.hidden);
  const end = card.querySelector(".ov-end");
  const shown = end && !end.hidden;
  const last = shown ? end : rows[rows.length - 1];
  const rel = document.querySelector(".ov-pair > .ov-rel");
  return [rows.length, shown ? 1 : 0, Math.round(
    card.getBoundingClientRect().bottom - last.getBoundingClientRect().bottom),
    Math.round(card.getBoundingClientRect().bottom),
    rel && !rel.hidden ? Math.round(rel.getBoundingClientRect().bottom) : 0,
  ].join("|")
})()`

// The issues card uses the height its neighbour sets: more rows when
// there is room, the end line when there are no more, the usual five
// on a phone and when no release went wrong.
func TestBrowserOverviewLookFillsItsRow(t *testing.T) {
	cases := []struct {
		name        string
		w           int64
		n, rels     int
		rows, end   int
		maxGap      int
		minRows     int
		sameBottoms bool
		exactRows   bool
	}{
		{"one-issue", 1440, 1, 9, 1, 1, 400, 1, true, true},
		{"many", 1440, 8, 7, 0, 0, 80, 6, true, false},
		{"many-wide", 2000, 8, 7, 0, 0, 80, 6, true, false},
		{"none-bad", 1440, 8, 0, 5, 0, 1 << 20, 5, false, true},
		{"phone", 390, 8, 9, 5, 0, 1 << 20, 5, false, true},
		{"phone-one", 390, 1, 9, 1, 0, 1 << 20, 1, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := listsServer(t, map[string]any{
				"/api/overview": fillAnswer(c.n, c.rels)})
			tab := pagingTab(t, srv, c.w, 900, "/", ".ov-look")
			time.Sleep(400 * time.Millisecond)
			got := strings.Split(evalString(t, tab, fillProbe), "|")
			rows, _ := strconv.Atoi(got[0])
			gap, _ := strconv.Atoi(got[2])
			if c.exactRows && rows != c.rows || rows < c.minRows {
				t.Errorf("%d rows shown, want %d (%v)", rows, c.rows, got)
			}
			if got[1] != fmt.Sprint(c.end) && c.name != "many" &&
				c.name != "many-wide" {
				t.Errorf("end line %s, want %d", got[1], c.end)
			}
			if gap > c.maxGap || gap < 0 {
				t.Errorf("%dpx empty under the last row (max %d): %v",
					gap, c.maxGap, got)
			}
			if c.sameBottoms && got[3] != got[4] {
				t.Errorf("card bottoms differ: %v", got)
			}
			if dir := os.Getenv("KW_SHOTS"); dir != "" {
				save(t, tab, filepath.Join(dir, fmt.Sprintf(
					"look-%s-%d.png", c.name, c.w)), true)
			}
		})
	}
}
