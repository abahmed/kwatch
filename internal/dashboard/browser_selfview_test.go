//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// A half-blind kwatch says so on its health page, one notice per part
// that does not work, counts them in the side bar's dot and never says
// it is working normally.
func TestBrowserHealthShowsBlindSpots(t *testing.T) {
	st := healthStatus(healthOpts{memory: 0.4})
	st["kwatch"].(map[string]any)["blindSpots"] = []any{
		map[string]any{"part": "kubelet-stats",
			"reason": "kubelet_unreachable"},
		map[string]any{"part": "license", "reason": "license_expired"}}
	srv := stubAPIServer(t, map[string]any{"/api/status": st})
	got := healthText(t, srv, 1280, `(() => [
  document.querySelector(".ov-say").textContent,
  [...document.querySelectorAll("[data-blind]")].
    map((n) => n.textContent).join(" / "),
  document.querySelector("[data-kw-dot]").hidden,
  document.querySelector("[data-kw-dot]").dataset.count].join("|"))()`)
	want := "kwatch is running, but 2 things need a look|" +
		"kwatch can't read node stats from the kubeletsNo kubelet " +
		"answers. Memory and CPU use may be missing on issue pages. / " +
		"The Pro license has expiredPro features are off.|false|2"
	if got != want {
		t.Errorf("health page says\n%q, want\n%q", got, want)
	}
}

// The About card says "Watching" once, and a kwatch still reading the
// cluster says so once: in the verdict, with its progress, not in a
// notice beside it.
func TestBrowserHealthSaysThingsOnce(t *testing.T) {
	st := healthStatus(healthOpts{memory: 0.4})
	st["self"].(map[string]any)["startup"] = map[string]any{
		"done": false, "kinds": 40, "synced": 12, "objects": 3200}
	delete(st["kwatch"].(map[string]any), "coverage")
	srv := stubAPIServer(t, map[string]any{"/api/status": st,
		"/api/coverage": map[string]any{}})
	got := healthText(t, srv, 1280, `(() => [
  document.querySelector(".kp-about").textContent.includes(
    "WatchingWatching"),
  document.querySelector(".kp-about").textContent.includes(
    "Watchingevery namespace"),
  [...document.querySelectorAll(".ov-notes .notice")].
    filter((n) => /loading|starting/i.test(n.textContent)).length,
  document.querySelector(".ov-say").textContent,
  document.querySelector(".ov-tile").textContent.includes(
    "12 of 40 kinds read")].join("|"))()`)
	if got != "false|true|0|kwatch is starting up|true" {
		t.Errorf("health page says %q", got)
	}
}

// The week's numbers count the issues someone was alerted about; the
// median says it is over those.
func TestBrowserOverviewWeekCountsAlerted(t *testing.T) {
	ov := overviewAnswer(1, 0, 0, false)
	for _, d := range ov["insights"].(map[string]any)["days"].([]map[string]any) {
		d["resolvedAlerted"] = 3
	}
	tr := ov["trends"].(map[string]any)
	tr["resolveAlerted"] = map[string]any{"count": 4, "medianSeconds": 600}
	tr["previousWeek"], tr["previousWeekAlerted"] = 60, 20
	srv := listsServer(t, map[string]any{"/api/overview": ov})
	tab := pagingTab(t, srv, 1280, 800, "/", ".ov-stat")
	got := evalString(t, tab, `[...document.querySelectorAll(".ov-stat")].
  map((d) => d.textContent).join("|")`)
	want := "Alerted211 more than last week|Resolved21of the alerted " +
		"issues|Alerted the on-call person7urgent issues opened|" +
		"Median time to resolve an alerted issue10 minover 4 alerted " +
		"issues resolved"
	if got != want {
		t.Errorf("week numbers\n%q, want\n%q", got, want)
	}
}

// The side bar, the tab title and KW.cluster name the cluster once
// signed in, with "Non-production" when it is set; an unnamed one says
// so. The sign-in page names no cluster.
func TestBrowserClusterNameAndBadge(t *testing.T) {
	st := healthStatus(healthOpts{memory: 0.4})
	st["cluster"] = "shop-eu"
	st["kwatch"].(map[string]any)["nonProduction"] = true
	srv := stubAPIServer(t, map[string]any{"/api/status": st})
	got := healthText(t, srv, 1280, `(() => [
  document.querySelector("[data-cluster-name]").textContent,
  document.querySelector("[data-cluster-env]").checkVisibility(),
  document.title, KW.cluster.name].join("|"))()`)
	want := "shop-eu|true|kwatch health - shop-eu (non-production) - " +
		"kwatch|shop-eu"
	if got != want {
		t.Errorf("cluster shows %q, want %q", got, want)
	}

	st = healthStatus(healthOpts{memory: 0.4})
	srv = stubAPIServer(t, map[string]any{"/api/status": st})
	got = healthText(t, srv, 390, `(() => [
  document.querySelector("[data-cluster-name]").textContent,
  document.querySelector("[data-cluster-env]").checkVisibility(),
  document.title].join("|"))()`)
	if got != "unnamed cluster|false|kwatch health - unnamed cluster - "+
		"kwatch" {
		t.Errorf("unnamed cluster shows %q", got)
	}

	actx, _ := browserSetup(t)
	tab, _ := browserTab(t, actx)
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS(".signin"))); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, tab, `document.title + "|" +
  document.querySelectorAll("[data-cluster]").length`); got !=
		"Sign in - kwatch|0" {
		t.Errorf("sign-in page shows %q", got)
	}
}

// An issue with no cause found labels none of its objects the likely
// cause.
func TestBrowserAffectedNoCauseLabel(t *testing.T) {
	srv := listsServer(t, nil)
	tab := pagingTab(t, srv, 1280, 800, "/issues/inc-005#impact",
		`[data-panel="impact"]`)
	got := evalString(t, tab, `document.querySelector(
  '[data-panel="impact"]').textContent`)
	if !strings.Contains(got, "app-inc-005") ||
		strings.Contains(got, "Likely cause") {
		t.Errorf("impact tab says %q", got)
	}
}

// A held issue says until when, beside its level and under "Who was
// alerted?".
func TestBrowserHeldIssueSaysUntilWhen(t *testing.T) {
	held := realAnswer(t, "/api/incidents/inc-004")
	held["state"] = "settling"
	held["why"] = map[string]any{"alertLevelReason": "Urgent: users " +
		"can't reach it.", "pagingHeldReason": "Paging held: cluster " +
		"is scaling", "pagingHeldUntil": time.Now().
		Add(20 * time.Minute).UTC().Format(time.RFC3339)}
	srv := listsServer(t, map[string]any{"/api/incidents/inc-004": held})
	tab := pagingTab(t, srv, 1280, 800, "/issues/inc-004",
		"[data-level-note]")
	got := evalString(t, tab, `document.querySelector(
  "[data-level-note]").textContent + "|" +
  document.querySelector("main").textContent`)
	note, page, _ := strings.Cut(got, "|")
	if !strings.HasPrefix(note, "Paging held: cluster is scaling · "+
		"until ~") || len(note) < 45 {
		t.Errorf("level note is %q", note)
	}
	if !strings.Contains(page, "Nobody was alerted yet: the page is "+
		"held while the cluster scales, until ~") {
		t.Errorf("who was alerted does not mention the hold: %q", page)
	}
}
