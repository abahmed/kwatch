//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// "What's wrong?" says first whether users are affected, then after
// the sentence why the members are one issue, with the members by
// group behind a fold, and what the failing replicas share. On a
// phone none of it runs off the screen.
func TestBrowserWhatsWrongShowsTheIssueShape(t *testing.T) {
	d := stubDetail()
	sum := d["summary"].(map[string]any)
	sum["users"] = map[string]any{"state": "affected",
		"text": "Users are affected: route shop.example.com → api " +
			"has 0 of 3 ready endpoints.", "ready": 0, "total": 3}
	sum["grouped"] = map[string]any{"basis": "node",
		"text": "Grouped because all 3 pods run on node " +
			"ip-10-0-1-23.example.internal, which is not ready.",
		"groups": []map[string]any{
			{"owner": map[string]any{"kind": "deployment",
				"namespace": "shop", "name": "api"},
				"members": []map[string]any{
					{"kind": "pod", "namespace": "shop", "name": "api-1"},
					{"kind": "pod", "namespace": "shop", "name": "api-2"}}},
			{"owner": map[string]any{"kind": "deployment",
				"namespace": "shop", "name": "web"},
				"members": []map[string]any{
					{"kind": "pod", "namespace": "shop", "name": "web-1"}},
				"more": 2}}}
	sum["replicas"] = []string{"Only the 3 pods on arm64 nodes fail; " +
		"the 4 pods on amd64 nodes are healthy."}
	actx, _ := browserSetup(t)
	srv := stubIssueServer(t, d)
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(srv.URL+"/issues/stub-1#summary"),
		chromedp.WaitReady(chromedp.CSS(".sh-users")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](
		`(() => {
		  const dd = document.querySelector(".sh-users").parentElement;
		  const wide = Array.from(dd.querySelectorAll("*")).some((e) =>
		    e.getBoundingClientRect().right >
		    document.documentElement.clientWidth + 1);
		  dd.querySelector(".sh-groups").open = true;
		  return [dd.firstElementChild.className,
		    dd.querySelector(".sh-users").textContent,
		    dd.querySelector(".sh-why").textContent,
		    dd.querySelector(".sh-groups summary").textContent,
		    Array.from(dd.querySelectorAll(".sh-list li"))
		      .map((li) => li.textContent).join(" / "),
		    dd.querySelector(".sh-split").textContent,
		    wide ? "overflows" : "fits"].join("\n");
		})()`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"sh-users sh-hit\n",
		"Users are affected: route shop.example.com → api has 0 of 3",
		"Grouped because all 3 pods run on node",
		"Members by group (2)",
		"deployment/api · pod/api-1, pod/api-2 / " +
			"deployment/web · pod/web-1, 2 more",
		"Only the 3 pods on arm64 nodes fail;",
		"\nfits"} {
		if !strings.Contains(got, want) {
			t.Errorf("What's wrong? lacks %q:\n%s", want, got)
		}
	}
	for _, p := range problems.take() {
		t.Error(p)
	}
}

// Without the shape the question shows none of its lines.
func TestBrowserWhatsWrongWithoutShape(t *testing.T) {
	actx, _ := browserSetup(t)
	srv := stubIssueServer(t, stubDetail())
	tab, problems := browserTab(t, actx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/issues/stub-1"),
		chromedp.WaitReady(chromedp.CSS(".ds-what")),
		chromedp.Sleep(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	n, err := chromedp.Run(tab, chromedp.Evaluate[int](
		`document.querySelectorAll(".sh-users, .sh-grouped, .sh-split")
		  .length`))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d shape lines without a shape", n)
	}
	for _, p := range problems.take() {
		t.Error(p)
	}
}
