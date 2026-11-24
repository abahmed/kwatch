//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Every command block on the issue page says which cluster it is for,
// once above a list of steps; an unnamed cluster links to naming it.
func TestBrowserCommandsNameTheCluster(t *testing.T) {
	srv := stubIssueServer(t, stubDetail())
	for _, w := range []int64{1280, 390} {
		text, done := polishPage(t, srv.URL, "/issues/stub-1#alerts",
			".ds-r-told .cmd", w)
		got := text(`(() => {
		  const cmds = [...document.querySelectorAll(".cmd-wrap")]
		    .filter((x) => x.offsetParent);
		  const ctx = cmds[0].querySelector(".cmd-ctx");
		  const link = ctx.querySelector("a");
		  return cmds.length + "|" + ctx.textContent + "|" +
		    (link ? link.getAttribute("href") : "") + "|" +
		    getComputedStyle(ctx).display; })()`)
		if !strings.Contains(got, "Run against this cluster — check "+
			"kubectl config current-context first.") ||
			!strings.Contains(got, "|/kwatch#setup|") ||
			strings.HasSuffix(got, "|none") {
			t.Errorf("%dpx: %q", w, got)
		}
		done()
	}
}

// The Issues table shows the gist of why under each title and who
// acknowledged it, and has a "Not acknowledged" quick view.
func TestBrowserIssuesGistAckAndQuickView(t *testing.T) {
	ans := listAnswer(false)
	items := ans["incidents"].([]any)
	first := items[0].(map[string]any)
	first["title"] = "api in external-secrets is down: its database " +
		"refuses connections"
	first["ackedBy"] = "Deployment/external-secrets/api"
	second := items[1].(map[string]any)
	second["rootCause"] = "ConfigMap api-config changed 2 minutes before"
	srv := listsServer(t, map[string]any{"/api/incidents": ans})
	for _, w := range []int64{1280, 390} {
		text, done := polishPage(t, srv.URL, "/issues", ".it-gist", w)
		got := text(`(() => {
		  const rows = [...document.querySelectorAll("#issues-table tbody tr")];
		  const g = (r) => (r.querySelector(".it-gist") || {}).textContent;
		  const ack = rows[0].querySelector(".i-acked");
		  const qv = document.querySelector('[data-qv="unacked"]');
		  return g(rows[0]) + "|" + g(rows[1]) + "|" +
		    (ack ? ack.textContent : "") + "|" + (qv ? qv.textContent : "");
		})()`)
		want := "its database refuses connections|ConfigMap api-config " +
			"changed 2 minutes before|Acknowledged|Not acknowledged"
		if got != want {
			t.Errorf("%dpx: %q, want %q", w, got, want)
		}
		done()
	}
}

// The quick view picks urgent and needs-attention issues nobody
// acknowledged, across the whole active list.
func TestBrowserNotAcknowledgedQuickView(t *testing.T) {
	srv := listsServer(t, nil)
	tab := pagingTab(t, srv, 1280, 800, "/issues", "#issues-table tbody tr")
	err := chromedp.Do(tab, chromedp.Click(".ftoggle"),
		chromedp.Click(`[data-qv="unacked"]`),
		chromedp.Sleep(800*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got := evalString(t, tab, `location.search + "|" +
	  document.querySelector('[data-bucket][aria-selected="true"]').
	    dataset.bucket`)
	for _, want := range []string{"severity=page", "severity=notify",
		"acked=false", "|all"} {
		if !strings.Contains(got, want) {
			t.Errorf("quick view: %q lacks %q", got, want)
		}
	}
}

// The active list is split into tabs with counts; Needs attention
// opens first, a tab is in the address, and a link with filters opens
// on all active issues as before.
func TestBrowserIssuesBucketTabs(t *testing.T) {
	srv := listsServer(t, nil)
	for _, w := range []int64{1280, 390} {
		tab := pagingTab(t, srv, w, 800, "/issues", "#issues-table tbody tr")
		state := `[...document.querySelectorAll("[data-bucket]")].map((b) =>
		  b.dataset.bucket + ":" + b.getAttribute("aria-selected") + ":" +
		  b.querySelector("[data-n]").textContent).join(",")`
		got := evalString(t, tab, state)
		want := "attention:true:40,ongoing:false:0,cleanup:false:0," +
			"all:false:40"
		if got != want {
			t.Errorf("%dpx tabs: %q, want %q", w, got, want)
		}
		err := chromedp.Do(tab, chromedp.Click(`[data-bucket="ongoing"]`),
			chromedp.WaitVisible(chromedp.CSS("[data-empty] .nomatch")))
		if err != nil {
			t.Fatal(err)
		}
		got = evalString(t, tab, `location.search + "|" +
		  document.querySelector("[data-empty]").textContent`)
		if !strings.Contains(got, "bucket=ongoing") ||
			!strings.Contains(got, "Nothing in Ongoing right now.") {
			t.Errorf("%dpx ongoing: %q", w, got)
		}
		err = chromedp.Do(tab,
			chromedp.Navigate(srv.URL+"/issues?status=active&severity=page"),
			chromedp.WaitVisible(chromedp.CSS("#issues-table tbody tr")),
			chromedp.Sleep(300*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		got = evalString(t, tab, `document.querySelector(
		  '[data-bucket][aria-selected="true"]').dataset.bucket + "|" +
		  document.querySelectorAll("#issues-table tbody tr").length`)
		if got != "all|10" {
			t.Errorf("%dpx deep link: %q, want all|10", w, got)
		}
	}
}

// Why? states what kwatch's own check between two causes decided.
func TestBrowserWhySaysWhatTheCheckRuledOut(t *testing.T) {
	d := stubDetail()
	d["round"] = map[string]any{
		"check": map[string]any{"text": "kwatch checked: db resolves " +
			"from kwatch and accepts a connection from kwatch.",
			"cause": "external-endpoint//db:5432",
			"rival": "cluster-dns//cluster-dns"},
		"candidates": []any{
			map[string]any{"cause": "external-endpoint//db:5432",
				"state": "kept", "score": 80},
			map[string]any{"cause": "cluster-dns//cluster-dns",
				"state": "ruled out", "score": 60}}}
	srv := stubIssueServer(t, d)
	text, done := polishPage(t, srv.URL, "/issues/stub-1", ".ds-r-why", 1280)
	defer done()
	got := text(`document.querySelector(".ds-r-why dd").textContent`)
	if !strings.Contains(got, "Likely external endpoint db:5432 — kwatch "+
		"ruled out cluster DNS: db resolves from kwatch and accepts a "+
		"connection from kwatch.") {
		t.Errorf("Why? lacks the verdict: %q", got)
	}
}

// Notes about different objects that say the same thing are one row
// with a count, not one row each.
func TestBrowserTimelineGroupsTheSameNoteAcrossObjects(t *testing.T) {
	d := stubDetail()
	var tl []any
	for i, name := range []string{"alpha", "beta", "gamma"} {
		at := time.Now().Add(-time.Duration(10+i) * time.Minute)
		tl = append(tl, map[string]any{"kind": "note",
			"at": at.UTC().Format(time.RFC3339), "entity": "pod/shop/" + name,
			"text": "deletion of pod " + name + " is blocked by finalizers"})
	}
	d["timeline"] = tl
	srv := stubIssueServer(t, d)
	text, done := polishPage(t, srv.URL, "/issues/stub-1#timeline",
		"[data-timeline] li.ev", 1280)
	defer done()
	got := text(`[...document.querySelectorAll("[data-timeline] li.ev")]
	  .filter((li) => /blocked by finalizers/.test(li.textContent))
	  .map((li) => li.querySelector(".tx-text").textContent).join("\n")`)
	if strings.Count(got, "\n") != 0 || !strings.Contains(got, "3 times") {
		t.Errorf("the same note is not one row: %q", got)
	}
}
