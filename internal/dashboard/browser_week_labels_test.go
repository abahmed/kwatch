//go:build browser

package dashboard

import (
	"fmt"
	"strings"
	"testing"
)

// weekLabelsProbe answers "shown/total|overlaps" for the Day by day
// column numbers: how many are drawn, and how many pairs touch.
const weekLabelsProbe = `(() => {
  const all = [...document.querySelectorAll(".ip-chart .viz-cnum")];
  const shown = all.filter((t) => getComputedStyle(t).display !== "none");
  const boxes = shown.map((t) => t.getBoundingClientRect());
  let hits = 0;
  for (let i = 0; i < boxes.length; i++) {
    for (let j = i + 1; j < boxes.length; j++) {
      const a = boxes[i], b = boxes[j];
      if (a.left < b.right && b.left < a.right &&
        a.top < b.bottom && b.top < a.bottom) hits++;
    }
  }
  return shown.length + "/" + all.length + "|" + hits +
    "|" + shown.map((t) => t.textContent).join(" ");
})()`

// The Day by day numbers never print on top of each other, on a phone
// or a wide screen, even with 4-digit counts; a wide screen shows them
// all.
func TestBrowserWeekLabelsNeverOverlap(t *testing.T) {
	ov := overviewAnswer(1, 0, 0, false)
	days := ov["insights"].(map[string]any)["days"].([]map[string]any)
	for i, d := range days {
		d["opened"], d["resolved"] = 1190+i, 1191+i
	}
	srv := listsServer(t, map[string]any{"/api/overview": ov})
	for _, w := range []int64{390, 1440} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			tab := pagingTab(t, srv, w, 900, "/", ".ip-chart svg")
			got := strings.SplitN(evalString(t, tab, weekLabelsProbe),
				"|", 3)
			if got[1] != "0" {
				t.Errorf("%s labels overlap: %s", got[1], got[2])
			}
			if w == 1440 && got[0] != "14/14" {
				t.Errorf("wide screen shows %s labels", got[0])
			}
			if got[2] != "" && !strings.Contains(got[2], "1,19") ||
				strings.Contains(got[2], "119") &&
					!strings.Contains(got[2], "1,19") {
				t.Errorf("labels lack separators: %s", got[2])
			}
		})
	}
}
