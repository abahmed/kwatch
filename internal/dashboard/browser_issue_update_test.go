//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// updateState says which tab is open, the leftover row's text, the
// scroll position, whether the waiting note shows, and whether a part
// is marked changed or "Updated just now" shows.
const updateState = `(() => {
  const tab = document.querySelector('[role=tab][aria-selected=true]');
  const wait = document.querySelector(".d-wait");
  const upd = document.querySelector(".d-upd");
  return [tab.dataset.tab,
    document.querySelector('[data-fold="leftovers"] .lo-what').textContent,
    Math.round(scrollY), wait && !wait.hidden ? "waiting" : "-",
    document.querySelector(".kw-changed") || (upd && !upd.hidden) ?
      "marked" : "-"].join("|");
})()`

// An issue that changed is drawn again by itself, in place: the tab and
// the scroll position stay and the change is marked. While text is
// selected it waits with a quiet note and applies once the selection
// goes.
func TestBrowserIssueUpdatesInPlace(t *testing.T) {
	srv := changingDetail(t)
	tab := pagingTab(t, srv, 1280, 600, "/issues/inc-003#impact",
		"[data-level-note]")
	evalString(t, tab, `(() => { scrollTo(0, 120); return ""; })()`)
	before := strings.Split(evalString(t, tab, updateState), "|")
	refreshPage(t, tab)
	after := strings.Split(evalString(t, tab, updateState), "|")
	if after[1] == before[1] {
		t.Fatalf("not redrawn: %v", after)
	}
	if after[0] != "impact" || after[2] != before[2] || after[3] != "-" ||
		after[4] != "marked" {
		t.Errorf("before %v, after %v: want the same tab and place, no "+
			"waiting note, the change marked", before, after)
	}
	// Text selected: the update waits.
	evalString(t, tab, `(() => {
  const r = document.createRange();
  r.selectNodeContents(document.querySelector("h1"));
  getSelection().removeAllRanges(); getSelection().addRange(r);
  return ""; })()`)
	held := strings.Split(evalString(t, tab, updateState), "|")
	refreshPage(t, tab)
	waiting := strings.Split(evalString(t, tab, updateState), "|")
	if waiting[1] != held[1] || waiting[3] != "waiting" {
		t.Errorf("with text selected: %v, want %q kept and the note",
			waiting, held[1])
	}
	evalString(t, tab, `(() => { getSelection().removeAllRanges();
  return ""; })()`)
	if err := chromedp.Do(tab,
		chromedp.Sleep(300*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	done := strings.Split(evalString(t, tab, updateState), "|")
	if done[1] == held[1] || done[3] != "-" {
		t.Errorf("after the selection went: %v, want it applied", done)
	}
}
