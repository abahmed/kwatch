//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// "What changed just before" measures every change from the onset the
// server sorted it by, so a change labelled Before never reads "after
// it started", even when the issue first opened hours earlier. An
// automatic actor is marked as one.
func TestBrowserChangesMeasureFromTheOnset(t *testing.T) {
	ago := func(d time.Duration) string {
		return time.Now().Add(-d).UTC().Format(time.RFC3339)
	}
	d := stubDetail()
	d["firstSeen"] = ago(10 * time.Hour)
	d["onset"] = ago(40 * time.Minute)
	d["changes"] = []map[string]any{
		{"at": ago(50 * time.Minute), "when": "before",
			"kind": "ConfigMap", "namespace": "shop", "name": "cfg",
			"summary": "key A changed", "actor": "kubectl-edit"},
		{"at": ago(20 * time.Minute), "when": "during",
			"kind": "Node", "name": "node-1", "summary": "deleted",
			"actor": "karpenter"}}
	actx, _ := browserSetup(t)
	srv := stubIssueServer(t, d)
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate(srv.URL+"/issues/stub-1#timeline"),
		chromedp.WaitReady(chromedp.CSS(".cx-row")),
		chromedp.Sleep(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](
		`Array.from(document.querySelectorAll(
		  ".cx-row, [data-changes] > .ev-cap")).
		  map((r) => r.textContent).join("\n")`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Before", "10 min before it started failing",
		"During", "20 min after it started failing",
		"by karpenter (automatic)",
		"Changes made to this object and what it uses",
		"30 minutes before it started failing"} {
		if !strings.Contains(got, want) {
			t.Errorf("changes lack %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"h 10 min after", "people made",
		"kubectl-edit (automatic)"} {
		if strings.Contains(got, bad) {
			t.Errorf("changes say %q:\n%s", bad, got)
		}
	}
	for _, p := range problems.take() {
		t.Error(p)
	}
}
