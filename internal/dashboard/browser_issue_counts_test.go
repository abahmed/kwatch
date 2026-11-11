//go:build browser

package dashboard

import (
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// sameMinuteHistory is n occurrences: the first four a few seconds
// apart in one minute, the rest a day apart.
func sameMinuteHistory(n int) []map[string]any {
	base := time.Now().Add(-48 * time.Hour).Truncate(time.Minute).
		Add(50 * time.Second)
	var out []map[string]any
	for i := 0; i < n; i++ {
		at := base.Add(-time.Duration(i) * 24 * time.Hour)
		if i < 4 {
			at = base.Add(-time.Duration(i) * 10 * time.Second)
		}
		out = append(out, map[string]any{
			"opened":   at.UTC().Format(time.RFC3339),
			"resolved": at.Add(2 * time.Second).UTC().Format(time.RFC3339),
			"fix":      "recovered without change", "mode": "Probe"})
	}
	return out
}

// The count in "Yes: 16 times" and the button agree, and times in the
// same minute are one line marked with their count in the preview and
// in the full list.
func TestBrowserSeenBeforeCountsAgree(t *testing.T) {
	actx, _ := browserSetup(t)
	detail := stubDetail()
	detail["seenBefore"] = map[string]any{"inWindow": 16, "windowDays": 30,
		"recent": sameMinuteHistory(16)}
	srv := stubIssueServer(t, detail)
	tab, _ := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	text := func(js string) string {
		got, err := chromedp.Run(tab, chromedp.Evaluate[string](js))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	err := chromedp.Do(tab, chromedp.EmulateViewport(390, 800),
		chromedp.Navigate(srv.URL+"/issues/stub-1#details"),
		chromedp.WaitVisible(chromedp.CSS(".seen-all")))
	if err != nil {
		t.Fatal(err)
	}
	if got := text(`document.querySelector(".seen-h").textContent`); got !=
		"Yes: 16 times in the last 30 days." {
		t.Errorf("headline: %q", got)
	}
	if got := text(`document.querySelector(".seen-all").textContent`); got !=
		"Show all 16" {
		t.Errorf("button: %q, want Show all 16", got)
	}
	if got := text(`document.querySelector(".seen-list li .seen-x").
	  textContent.trim()`); got != "×4" {
		t.Errorf("preview duplicate mark: %q", got)
	}
	if err := chromedp.Do(tab, chromedp.Click(".seen-all"),
		chromedp.Sleep(500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if got := text(`String(document.querySelectorAll(
	  ".seen-list li").length) + "|" + document.querySelector(
	  ".seen-list .seen-x").textContent.trim()`); got != "13|×4" {
		t.Errorf("full list: %q, want 13 lines with ×4", got)
	}
}
