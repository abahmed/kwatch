//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// Each change in "What changed just before" carries the server's
// sentence on why it is listed, muted under its line; a change the
// server gave no reason says nothing more. On a phone the sentence
// wraps inside its row.
func TestBrowserChangesSayWhyEachIsShown(t *testing.T) {
	ago := func(d time.Duration) string {
		return time.Now().Add(-d).UTC().Format(time.RFC3339)
	}
	d := stubDetail()
	d["onset"] = ago(40 * time.Minute)
	d["changes"] = []map[string]any{
		{"at": ago(50 * time.Minute), "when": "before",
			"kind": "ConfigMap", "namespace": "shop", "name": "cfg",
			"summary": "key A changed", "actor": "kubectl-edit",
			"why": "The failing pods mount this ConfigMap."},
		{"at": ago(45 * time.Minute), "when": "before",
			"kind": "ConfigMap", "namespace": "shop", "name": "other",
			"summary": "key B changed", "actor": "kubectl-edit"}}
	actx, _ := browserSetup(t)
	srv := stubIssueServer(t, d)
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(srv.URL+"/issues/stub-1#timeline"),
		chromedp.WaitReady(chromedp.CSS(".cx-row")),
		chromedp.Sleep(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](
		`Array.from(document.querySelectorAll(".cx-row")).map((r) => {
		  const w = r.querySelector(".cx-why");
		  if (!w) return r.querySelector(".cx-line").textContent +
		    " | -";
		  const wide = w.scrollWidth > w.clientWidth + 1 ||
		    w.getBoundingClientRect().right >
		    document.documentElement.clientWidth;
		  return r.querySelector(".cx-line").textContent + " | " +
		    w.textContent + (wide ? " | overflows" : "");
		}).join("\n")`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"cfg: key A changed | The failing pods mount this ConfigMap.",
		"other: key B changed | -"} {
		if !strings.Contains(got, want) {
			t.Errorf("changes lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "overflows") {
		t.Errorf("a reason overflows its row on a phone:\n%s", got)
	}
	for _, p := range problems.take() {
		t.Error(p)
	}
}
