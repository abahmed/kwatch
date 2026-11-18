//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// An issue without a cause lists what kwatch checked for it under
// Why?, and what it could not check; a change links to its source,
// and a link without an address is text. Nothing overflows a phone.
func TestBrowserWhySaysWhatWasCheckedAndChangesLinkSources(t *testing.T) {
	ago := func(d time.Duration) string {
		return time.Now().Add(-d).UTC().Format(time.RFC3339)
	}
	d := stubDetail()
	// A release that went wrong is the cause the page states instead.
	delete(d, "release")
	sum := d["summary"].(map[string]any)
	sum["why"] = map[string]any{"text": "", "causeKnown": false,
		"checked": map[string]any{
			"fine": []string{"its node is healthy",
				"its ConfigMaps are there and did not change"},
			"unchecked": []string{"kwatch cannot read pod logs here"}}}
	d["onset"] = ago(40 * time.Minute)
	d["changes"] = []map[string]any{{"at": ago(50 * time.Minute),
		"when": "before", "kind": "Deployment", "namespace": "shop",
		"name": "api", "summary": "image api:1.4 → api:1.5",
		"links": []map[string]any{
			{"label": "Argo CD app shop",
				"url": "https://gitops.example.com/applications/shop"},
			{"label": "tag v1.5.0"},
			{"label": "bad", "url": "javascript:alert(1)"}}}}
	actx, _ := browserSetup(t)
	srv := stubIssueServer(t, d)
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(srv.URL+"/issues/stub-1#summary"),
		chromedp.WaitReady(chromedp.CSS(".ds-checked")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	why, err := chromedp.Run(tab, chromedp.Evaluate[string](
		`document.querySelector(".ds-checked").innerText`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kwatch checked:", "its node is healthy",
		"its ConfigMaps are there and did not change",
		"kwatch cannot read pod logs here."} {
		if !strings.Contains(why, want) {
			t.Errorf("Why? lacks %q:\n%s", want, why)
		}
	}
	err = chromedp.Do(tab, chromedp.Navigate(srv.URL+"/issues/stub-1#timeline"),
		chromedp.WaitReady(chromedp.CSS(".cx-src")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	src, err := chromedp.Run(tab, chromedp.Evaluate[string](
		`(() => { const p = document.querySelector(".cx-src");
		  const links = Array.from(p.querySelectorAll("a")).map((a) =>
		    a.textContent + "=" + a.getAttribute("href") + "/" + a.rel);
		  const wide = p.getBoundingClientRect().right >
		    document.documentElement.clientWidth;
		  return p.textContent + " | " + links.join(",") +
		    (wide ? " | overflows" : ""); })()`))
	if err != nil {
		t.Fatal(err)
	}
	want := "Source: Argo CD app shop · tag v1.5.0 · bad | Argo CD app " +
		"shop=https://gitops.example.com/applications/shop/noopener " +
		"noreferrer"
	if src != want {
		t.Errorf("source line = %q, want %q", src, want)
	}
	for _, p := range problems.take() {
		t.Error(p)
	}
}
