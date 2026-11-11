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

// stubIssueServer serves the dashboard with /api/incidents/stub-1
// answered by detail, so the issue page is drawn from a fixed answer.
func stubIssueServer(t *testing.T, detail map[string]any) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	inner := newTestServer(browserIssues()...).Handler()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/incidents/stub-1" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
				return
			}
			inner.ServeHTTP(w, r)
		}))
	t.Cleanup(srv.Close)
	return srv
}

const stubTitle = "api in shop: the new version can't start; the " +
	"previous version is still serving: it stops at start with " +
	"\"Unable to resolve service for type Example.IClient while " +
	"attempting to activate Example.Worker\" and keeps restarting"

// stubDetail is an issue with a release, an undo, repeated readiness
// failures and earlier occurrences, named neutrally.
func stubDetail() map[string]any {
	ago := func(d time.Duration) string {
		return time.Now().Add(-d).UTC().Format(time.RFC3339)
	}
	var tl []map[string]any
	for i := 0; i < 6; i++ {
		d := time.Duration(i) * 7 * time.Minute
		tl = append(tl, map[string]any{"at": ago(80*time.Minute - d),
			"kind": "note", "entity": "pod/shop/api-1",
			"text": "Container keeps failing its readiness probe: " +
				"12 failures, last: connection refused " +
				"(container shop/api-1/app)"})
		tl = append(tl, map[string]any{"at": ago(78*time.Minute - d),
			"kind": "note", "entity": "pod/shop/api-1",
			"text": "recovered: Container keeps failing its " +
				"readiness probe (container shop/api-1/app)"})
	}
	var recent []map[string]any
	for i := 1; i <= 7; i++ {
		fix := "recovered without change"
		if i == 3 {
			fix = "change"
		}
		recent = append(recent, map[string]any{
			"opened":   ago(time.Duration(i) * 30 * time.Hour),
			"resolved": ago(time.Duration(i)*30*time.Hour - 8*time.Minute),
			"fix":      fix, "mode": "Probe"})
	}
	return map[string]any{
		"id": "stub-1", "title": stubTitle[:90] + "…",
		"fullTitle": stubTitle, "severity": "notify",
		"state": "open", "namespace": "shop", "workload": "api",
		"workloadKind": "Deployment", "firstSeen": ago(3 * time.Hour),
		"lastSeen": ago(3 * time.Hour), "timeline": tl,
		"fixSteps": []map[string]any{{"text": "See the object's state",
			"command": "kubectl describe deployment api -n shop"}},
		"summary": map[string]any{
			"what": "The new version of api crashes every time it " +
				"starts. The previous version is still serving users.",
			"why": map[string]any{"text": "deployment api explains 4 failures",
				"confidence": "high", "causeKnown": true},
			"proof": []map[string]any{{"kind": "log",
				"text":   "Last error in the logs",
				"detail": "--- End of inner exception stack trace ---"},
				{"kind": "exit", "text": "Container app stopped",
					"detail": "Error (exit code 143)", "count": 95}},
			"told":  []map[string]any{},
			"stage": map[string]any{"step": "happening"}},
		"capturedLogLines": []string{
			"Unhandled exception. System.AggregateException: Some failed",
			"---> System.InvalidOperationException: Unable to resolve " +
				"service for type 'Example.IClient'",
			"--- End of inner exception stack trace ---"},
		"release": map[string]any{"id": "r", "kind": "deployment",
			"namespace": "shop", "name": "api", "at": ago(181 * time.Minute),
			"verdict": "failed", "final": true, "app": "argocd/api",
			"images": []map[string]any{{"container": "app",
				"before": "registry.example.com/team/api:1.4",
				"after":  "registry.example.com/team/api:1.5"}},
			"detail": map[string]any{"restartsBefore": 0, "restartsAfter": 0},
			"undo": map[string]any{"text": "Revert the change in Git " +
				"(argocd/api): a rollback in the cluster would be " +
				"overwritten by the next sync"}},
		"seenBefore": map[string]any{"inWindow": 7, "windowDays": 30,
			"recent": recent},
		"similar": 5,
		"similarIssues": []map[string]any{
			{"id": "inc-002", "title": "worker in shop is not ready", "state": "open"},
			{"id": "inc-003", "title": "web in shop is not ready", "state": "resolved"}},
		"timing": map[string]any{"startedAt": ago(3 * time.Hour),
			"noticedAt": ago(179 * time.Minute),
			"toldAt":    ago(172 * time.Minute), "toldVia": "Slack"},
	}
}

func TestBrowserIssuePageIsPlain(t *testing.T) {
	actx, _ := browserSetup(t)
	srv := stubIssueServer(t, stubDetail())
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	text := func(js string) string {
		got, err := chromedp.Run(tab, chromedp.Evaluate[string](js))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate(srv.URL+"/issues/stub-1#timeline"),
		chromedp.WaitReady(chromedp.CSS(".ds-r-why")),
		chromedp.WaitVisible(chromedp.CSS("[data-panel=timeline]")),
		chromedp.Sleep(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	// The heading is the short title, not cut by CSS; the whole title
	// is in its tooltip (see TestBrowserIssueHeadingIsShort).
	got := text(`(() => { const h = document.querySelector("h1");
	  return h.title + "|" + (h.scrollHeight <= h.clientHeight + 1); })()`)
	if got != stubTitle+"|true" {
		t.Errorf("title is cut or changed: %q", got)
	}

	// Why? says the release in words, never the engine's sentence, and
	// quotes the line that names the error.
	why := text(`document.querySelector(".ds-r-why dd").textContent`)
	for _, want := range []string{"after the release", "image 1.4 → 1.5",
		"Unable to resolve service for type", "restarted 95 times"} {
		if !strings.Contains(why, want) {
			t.Errorf("Why? lacks %q: %q", want, why)
		}
	}
	for _, bad := range []string{"explains", "End of inner exception",
		"registry.example.com"} {
		if strings.Contains(why, bad) {
			t.Errorf("Why? shows %q: %q", bad, why)
		}
	}

	// What should I do? starts with the release's undo.
	first := text(`document.querySelector(".ds-r-next li").textContent`)
	if !strings.HasPrefix(first, "Revert the change in Git") {
		t.Errorf("first action is %q", first)
	}

	// The timeline puts what changed first and groups repeats.
	tl := text(`document.querySelector("[data-panel=timeline]").textContent`)
	if !strings.Contains(tl, "12 times between") &&
		!strings.Contains(tl, "6 times between") {
		t.Errorf("timeline repeats are not grouped: %q", tl)
	}
	if n := text(`String(Array.from(document.querySelectorAll(
	  "[data-timeline] li.ev")).filter((li) =>
	  /readiness/.test(li.textContent)).length)`); n != "1" {
		t.Errorf("%s readiness rows, want the one group", n)
	}
	if text(`document.querySelector("[data-panel=timeline] .sec-h").
	  textContent`) == "What happened, step by step" {
		t.Error("what changed just before is not first")
	}

	// The Details tab answers its three questions.
	if err := chromedp.Do(tab, chromedp.Navigate(srv.URL+
		"/issues/stub-1#details"), chromedp.Sleep(time.Second)); err != nil {
		t.Fatal(err)
	}
	heads := text(`Array.from(document.querySelectorAll(
	  "[data-panel=details] .sec:not([hidden]) .sec-h")).
	  map((h) => h.textContent).join("|")`)
	if heads != "Has this happened before?|Is it happening anywhere else?|"+
		"Reference" {
		t.Errorf("details sections: %q", heads)
	}
	more := text(`document.querySelector("[data-panel=details]").textContent`)
	for _, want := range []string{"Fixed by a change once",
		"worker in shop is not ready"} {
		if !strings.Contains(more, want) {
			t.Errorf("details lack %q", want)
		}
	}
	for _, p := range problems.take() {
		t.Error(p)
	}
}

// The issue page on a phone never scrolls sideways, on any tab.
func TestBrowserIssuePagePhoneFits(t *testing.T) {
	actx, _ := browserSetup(t)
	srv := stubIssueServer(t, stubDetail())
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	for _, key := range []string{"summary", "timeline", "evidence",
		"impact", "alerts", "details"} {
		err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
			chromedp.Navigate(srv.URL+"/issues/stub-1#"+key),
			chromedp.WaitVisible(chromedp.CSS("[data-panel="+key+"]")),
			chromedp.Sleep(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		over, err := chromedp.Run(tab,
			chromedp.Evaluate[float64](overflowScript))
		if err != nil {
			t.Fatal(err)
		}
		if over > 1 {
			t.Errorf("#%s scrolls sideways by %.0fpx", key, over)
		}
	}
}
