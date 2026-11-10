//go:build browser

package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// stubAPIServer serves the dashboard with some /api paths answered by
// fixed bodies, so a page is drawn from a known answer.
func stubAPIServer(t *testing.T, answers map[string]any) *httptest.Server {
	t.Helper()
	bodies := map[string][]byte{}
	for path, v := range answers {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		bodies[path] = b
	}
	inner := newTestServer(browserIssues()...).Handler()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if b, ok := bodies[r.URL.Path]; ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(b)
				return
			}
			inner.ServeHTTP(w, r)
		}))
	t.Cleanup(srv.Close)
	return srv
}

func isoAgo(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format(time.RFC3339)
}

// healthOpts changes the status a test starts from.
type healthOpts struct {
	memory  float64 // memory used, as a fraction of its limit
	failing bool    // the Slack channel's last send failed
	waiting int     // alerts waiting to be sent
}

// healthStatus is an /api/status answer for a kwatch that is well.
func healthStatus(o healthOpts) map[string]any {
	const limit = 536870912
	slack := map[string]any{"name": "Slack", "provider": "slack",
		"kind": "chat", "sent": 22, "failed": 0, "retrying": 0,
		"lastSuccess": isoAgo(9 * time.Minute)}
	if o.failing {
		slack["failed"] = 3
		slack["lastError"] = isoAgo(time.Minute)
	}
	return map[string]any{"version": "v1.2.0", "edition": "pro",
		"lastScan": isoAgo(time.Minute), "kwatchNamespace": "kwatch",
		"watchedNamespaces": []string{"*"},
		"self": map[string]any{"startedAt": isoAgo(4 * time.Hour),
			"lastApiContact": isoAgo(5 * time.Second),
			"outboxPending":  o.waiting,
			"startup":        map[string]any{"done": true},
			"dashboardLinks": map[string]any{"on": true}},
		"kwatch": map[string]any{
			"coverage": map[string]any{"watched": 233, "synced": 233,
				"total": 233, "allNamespaces": true, "namespaces": 24,
				"syncedAt": isoAgo(4 * time.Hour)},
			"delivery": []map[string]any{slack},
			"stateFile": map[string]any{"bytes": 169955328,
				"capBytes": limit, "capKnown": true},
			"resources": map[string]any{
				"memory": map[string]any{
					"used": int64(o.memory * limit), "limit": limit},
				"stateFile": map[string]any{"bytes": 169955328},
				"cpu":       map[string]any{"cores": 0.01, "limitCores": 0.5},
				"disk":      map[string]any{"used": 169979904, "total": 2040373248}}}}
}

// healthText opens /kwatch on srv and answers script's value.
func healthText(
	t *testing.T, srv *httptest.Server, w int64, script string,
) string {
	t.Helper()
	actx, _ := browserSetup(t)
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(w, 900),
		chromedp.Navigate(srv.URL+"/kwatch"),
		chromedp.WaitVisible(chromedp.CSS(".ov-say")),
		chromedp.Sleep(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](script))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

const healthScript = `(() => JSON.stringify({
  say: document.querySelector(".ov-say").textContent,
  tile: document.querySelector(".ov-tile").className,
  page: document.querySelector("main, #app").textContent,
  warn: document.querySelectorAll(".kp-res-part .viz-gfill.warn").length,
  bad: document.querySelectorAll(".kp-res-part .viz-gfill.bad").length,
  level: Array.from(document.querySelectorAll(".kp-res-h .chip")).
    map((c) => c.textContent).join("|")
}))()`

type healthSeen struct {
	Say, Tile, Page, Level string
	Warn, Bad              int
}

func seeHealth(t *testing.T, o healthOpts) healthSeen {
	t.Helper()
	srv := stubAPIServer(t, map[string]any{"/api/status": healthStatus(o)})
	var seen healthSeen
	if err := json.Unmarshal([]byte(healthText(t, srv, 1280, healthScript)),
		&seen); err != nil {
		t.Fatal(err)
	}
	return seen
}

// A well kwatch opens with one plain sentence, says what shows it, and
// leaves out every zero counter and the unexplained "Chat" chip.
func TestBrowserHealthHeadlineAndNoZeroCounters(t *testing.T) {
	seen := seeHealth(t, healthOpts{memory: 0.4})
	if seen.Say != "kwatch is working normally" ||
		!strings.Contains(seen.Tile, "ok") {
		t.Errorf("headline = %q (%s)", seen.Say, seen.Tile)
	}
	for _, want := range []string{"Last heard from the Kubernetes API",
		"Watching 233 of 233 resource types", "22 sent in the last 24 hours",
		"Memory", "includes its saved history"} {
		if !strings.Contains(strings.ToLower(seen.Page), strings.ToLower(want)) {
			t.Errorf("page lacks %q in %q", want, seen.Page)
		}
	}
	for _, bad := range []string{"Alerts waiting to be sent", "can't be read",
		"not read yet", "not watched", "0 failed", "Chat", "mapped into memory",
		"Messages waiting", "kinds of objects"} {
		if strings.Contains(seen.Page, bad) {
			t.Errorf("a healthy page shows %q", bad)
		}
	}
	if seen.Warn+seen.Bad != 0 {
		t.Errorf("calm gauges are %d amber, %d red", seen.Warn, seen.Bad)
	}
}

// A channel that fails, or alerts that wait, say so in plain words.
func TestBrowserHealthSaysWhatIsWrong(t *testing.T) {
	seen := seeHealth(t, healthOpts{memory: 0.4, failing: true, waiting: 3})
	if !strings.Contains(seen.Say, "alerts aren't getting out") ||
		!strings.Contains(seen.Tile, "bad") {
		t.Errorf("degraded headline = %q (%s)", seen.Say, seen.Tile)
	}
	if !strings.Contains(seen.Page, "Alerts waiting to be sent3") {
		t.Errorf("waiting alerts are not shown")
	}
}

// Memory is amber from 70% of its limit and red from 90%, in words as
// well as color, and the headline stops saying all is normal.
func TestBrowserHealthMemoryTurnsAmberThenRed(t *testing.T) {
	seen := seeHealth(t, healthOpts{memory: 0.74})
	if seen.Warn != 1 || seen.Bad != 0 || seen.Level != "Getting high" {
		t.Errorf("74%%: %d amber, %d red, %q", seen.Warn, seen.Bad, seen.Level)
	}
	if !strings.Contains(seen.Tile, "warn") ||
		seen.Say == "kwatch is working normally" {
		t.Errorf("74%% headline = %q (%s)", seen.Say, seen.Tile)
	}
	seen = seeHealth(t, healthOpts{memory: 0.93})
	if seen.Bad != 1 || seen.Level != "Almost full" ||
		!strings.Contains(seen.Tile, "bad") {
		t.Errorf("93%%: %d red, %q (%s)", seen.Bad, seen.Level, seen.Tile)
	}
}

// activityAnswer is an /api/activity answer: one issue resolved after
// 12 minutes, one that started, and a burst of six resolves in one
// minute.
func activityAnswer() map[string]any {
	minute := time.Now().Truncate(time.Minute).Add(-30 * time.Minute)
	ev := func(at time.Time, kind, id, workload string,
		extra map[string]any) map[string]any {
		e := map[string]any{"at": at.UTC().Format(time.RFC3339), "kind": kind,
			"id": id, "title": workload + " in shop keeps failing its liveness probe.",
			"severity": "digest", "state": "resolved", "namespace": "shop",
			"workload": workload, "workloadKind": "Deployment"}
		for k, v := range extra {
			e[k] = v
		}
		return e
	}
	events := []map[string]any{
		ev(time.Now().Add(-5*time.Minute), "new", "n1", "api",
			map[string]any{"severity": "notify", "state": "open"}),
		ev(time.Now().Add(-10*time.Minute), "resolved", "r0", "web",
			map[string]any{"lasted": 12 * 60})}
	for i, name := range []string{"a", "b", "c", "d", "e", "f"} {
		events = append(events, ev(minute.Add(time.Duration(i*5)*time.Second),
			"resolved", "b"+name, "svc-"+name, nil))
	}
	return map[string]any{"since": "1h", "events": events, "total": len(events),
		"kinds": map[string]int{"new": 1, "resolved": 7}, "untold": 0}
}

const activityScript = `(() => JSON.stringify({
  sum: document.querySelector(".act-sum").textContent,
  rows: Array.from(document.querySelectorAll(".act-list > li > .act-row")).
    map((a) => a.querySelector(".act-line").textContent),
  folds: Array.from(document.querySelectorAll(".act-fold > summary")).
    map((s) => s.textContent),
  folded: document.querySelectorAll(".act-sub .act-row").length
}))()`

// Each Activity row reads as a sentence, the totals are in words, and
// a burst of the same change in one minute folds behind one line.
func TestBrowserActivityRowsAreSentencesAndBurstsFold(t *testing.T) {
	actx, _ := browserSetup(t)
	srv := stubAPIServer(t, map[string]any{"/api/activity": activityAnswer()})
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate(srv.URL+"/activity"),
		chromedp.WaitVisible(chromedp.CSS(".act-row")),
		chromedp.Sleep(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := chromedp.Run(tab, chromedp.Evaluate[string](activityScript))
	if err != nil {
		t.Fatal(err)
	}
	var seen struct {
		Sum    string
		Rows   []string
		Folds  []string
		Folded int
	}
	if err := json.Unmarshal([]byte(raw), &seen); err != nil {
		t.Fatal(err)
	}
	if seen.Sum != "In the last hour, 1 issue started and 7 resolved." {
		t.Errorf("totals = %q", seen.Sum)
	}
	joined := strings.Join(seen.Rows, "\n")
	for _, want := range []string{"api in shop: started failing",
		"web in shop: resolved after 12 min"} {
		if !strings.Contains(joined, want) {
			t.Errorf("rows lack %q:\n%s", want, joined)
		}
	}
	// Six resolves in one minute: two shown, four folded.
	if len(seen.Folds) != 1 || seen.Folded != 4 ||
		!strings.HasPrefix(seen.Folds[0], "4 more resolved at ") {
		t.Errorf("folds = %q, %d folded rows", seen.Folds, seen.Folded)
	}
	if len(seen.Rows) != 4 {
		t.Errorf("%d rows shown, want 4: %q", len(seen.Rows), seen.Rows)
	}
}

const signInScript = `(() => JSON.stringify({
  h1: document.querySelector("h1").textContent,
  code: document.querySelectorAll("code, pre, .cmd").length,
  curl: document.body.textContent.includes("curl"),
  link: (document.querySelector(".signin .help a") || {}).textContent,
  href: (document.querySelector(".signin .help a") || {}).href || "",
  over: document.documentElement.scrollWidth - innerWidth
}))()`

// The sign-in page is a short message and a password field: no code
// block, a link to how to get the password, and no sideways scroll on
// a phone.
func TestBrowserSignInHasNoCodeBlock(t *testing.T) {
	actx, srv := browserSetup(t)
	tab, _ := browserTab(t, actx)
	err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(srv.URL+"/signin"),
		chromedp.WaitVisible(chromedp.CSS("#pw")),
		chromedp.Sleep(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](signInScript))
	if err != nil {
		t.Fatal(err)
	}
	var seen struct {
		H1, Link, Href string
		Code, Over     int
		Curl           bool
	}
	if err := json.Unmarshal([]byte(got), &seen); err != nil {
		t.Fatal(err)
	}
	if seen.H1 != "Sign in to see this cluster's issues" ||
		seen.Code != 0 || seen.Curl || seen.Over > 1 ||
		seen.Link != "How to get the password" ||
		!strings.HasSuffix(seen.Href, "docs/dashboard.md#how-to-get-the-password") {
		t.Errorf("sign-in page: %+v", seen)
	}
}
