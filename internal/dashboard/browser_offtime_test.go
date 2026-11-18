//go:build browser

package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// offOverview is an /api/overview answer whose week has one day kwatch
// was off all along and one it was off for 6 hours.
func offOverview() map[string]any {
	ov := overviewAnswer(1, 0, 0, false)
	days := ov["insights"].(map[string]any)["days"].([]map[string]any)
	for _, d := range days {
		d["seconds"] = 86400
	}
	days[1]["offSeconds"] = 86400
	days[1]["opened"], days[1]["resolved"] = 0, 0
	days[3]["offSeconds"] = 6 * 3600
	return ov
}

const offChartProbe = `(() => {
  const all = document.querySelectorAll(".ip-chart [data-off=all]");
  const part = document.querySelectorAll(".ip-chart [data-off=part]");
  const labels = [...document.querySelectorAll(".ip-chart .viz-off-t")]
    .map((t) => t.textContent).join(",");
  const wide = document.documentElement.scrollWidth > innerWidth;
  const w = document.querySelector(".ov-watching");
  return all.length + "|" + part.length + "|" + labels + "|" + wide +
    "|" + (w ? w.textContent : "") + "|" +
    document.querySelector(".ip-chart svg").getAttribute("aria-label");
})()`

// The week chart hatches the days kwatch was not running and says so
// in words, on a phone and a wide screen; the table says the same, and
// the week's numbers say how many days they count.
func TestBrowserWeekChartMarksDaysKwatchWasOff(t *testing.T) {
	srv := listsServer(t, map[string]any{"/api/overview": offOverview()})
	for _, w := range []int64{390, 1440} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			tab := pagingTab(t, srv, w, 900, "/", ".ip-chart svg")
			got := strings.Split(evalString(t, tab, offChartProbe), "|")
			if got[0] != "1" || got[1] != "1" {
				t.Errorf("marked days: all %s, part %s", got[0], got[1])
			}
			if got[2] != "off,6 h off" {
				t.Errorf("day labels = %q", got[2])
			}
			if got[3] != "false" {
				t.Error("the page scrolls sideways")
			}
			if !strings.Contains(got[4], "Based on 6 of 7 days of watching") {
				t.Errorf("week line = %q", got[4])
			}
			if !strings.Contains(got[5], "kwatch was not running") ||
				!strings.Contains(got[5], "kwatch was off for 6 h") {
				t.Errorf("chart label = %q", got[5])
			}
			cells := evalString(t, tab, `(() => {
  document.querySelector(".ip-view [data-view=table]").click();
  return [...document.querySelectorAll("td.ip-off")]
    .map((c) => c.textContent).join(",");
})()`)
			if cells != ",All day,,6 h,,," {
				t.Errorf("table off column = %q", cells)
			}
		})
	}
}

// Activity puts a marker where kwatch was not running, between the
// changes before and after it.
func TestBrowserActivityMarksTimeKwatchWasOff(t *testing.T) {
	ev := func(id string, ago time.Duration) map[string]any {
		return map[string]any{"at": listAgo(ago), "kind": "new",
			"id": id, "title": "api in shop is not ready",
			"severity": "notify", "state": "open", "namespace": "shop",
			"workload": "api"}
	}
	act := map[string]any{"since": "24h", "total": 2,
		"events": []any{ev("inc-001", time.Hour), ev("inc-002", 9*time.Hour)},
		"kinds":  map[string]any{"new": 2},
		"notWatching": []any{map[string]any{
			"from": listAgo(8 * time.Hour), "to": listAgo(2 * time.Hour)}}}
	srv := listsServer(t, map[string]any{"/api/activity": act})
	tab := pagingTab(t, srv, 390, 900, "/activity?since=24h", ".act-list")
	got := evalString(t, tab, `[...document.querySelectorAll(
  ".act-list > li")].map((li) => li.classList.contains("off-mark") ?
  "OFF:" + li.textContent : "row").join("|")`)
	parts := strings.Split(got, "|")
	if len(parts) != 3 || parts[0] != "row" || parts[2] != "row" ||
		!strings.Contains(parts[1], "kwatch was not running from") ||
		!strings.Contains(parts[1], "(6 h)") {
		t.Errorf("activity rows = %q", got)
	}
}

// The kwatch page always says whether anyone is told when kwatch
// stops: not set up (information, not a warning) or failing (why).
func TestBrowserHealthShowsTheHeartbeat(t *testing.T) {
	st := spotStatus("", 0, 0)
	kw := st["kwatch"].(map[string]any)
	kw["heartbeat"] = map[string]any{"state": "not_configured"}
	page := seeSpot(t, st)
	for _, want := range []string{"Heartbeat",
		"Nobody is told when kwatch itself stops.",
		"Set heartbeatMonitor.url"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	kw["heartbeat"] = map[string]any{"state": "failing",
		"failure": "rejected", "httpStatus": 404,
		"lastFailure": isoAgo(time.Minute), "lastPing": isoAgo(time.Hour)}
	page = seeSpot(t, st)
	for _, want := range []string{
		"The monitoring URL refused the ping (HTTP 404)",
		"failing since", "last good ping"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	kw["heartbeat"] = map[string]any{"state": "pro_only"}
	page = seeSpot(t, st)
	if !strings.Contains(page, "watch its pod from outside") ||
		strings.Contains(page, "heartbeatMonitor") {
		t.Errorf("community words: %q", page)
	}
}
