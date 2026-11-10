//go:build browser

package dashboard

import (
	"testing"

	"github.com/chromedp/chromedp"
)

// checkScript restarts the notification watcher three times; each start
// runs one check. The first check's read throws, the second's rejects,
// the third's succeeds. It answers how many reads each check made,
// joined by "|": a check that failed must not block the next one.
const checkScript = `(async () => {
  const wait = () => new Promise((r) => setTimeout(r, 100));
  const realApi = KW.api;
  KW.notify.stop();
  await wait();
  let phase = 0, calls = 0;
  KW.api = () => {
    calls++;
    if (phase === 1) throw new Error("read threw");
    if (phase === 2) return Promise.reject(new Error("read failed"));
    return Promise.resolve({ status: 200, etag: "",
      data: { incidents: [], totalPages: 1 } });
  };
  const counts = [];
  for (phase = 1; phase <= 3; phase++) {
    calls = 0;
    try { KW.notify.start(document.body); } catch (e) { /* counted */ }
    await wait();
    counts.push(calls);
    KW.notify.stop();
  }
  KW.api = realApi;
  return counts.join("|");
})()`

// A notification check whose read throws or rejects must reset its
// busy latch, so the next check still runs.
func TestBrowserNotifyCheckSurvivesFailedReads(t *testing.T) {
	ctx, srv := browserSetup(t)
	tab, _ := browserTab(t, ctx)
	signInBrowser(t, tab, srv.URL)
	err := chromedp.Do(tab, chromedp.Navigate(srv.URL+"/"),
		chromedp.WaitVisible(chromedp.CSS("h1")))
	if err != nil {
		t.Fatal(err)
	}
	got, err := chromedp.Run(tab, chromedp.Evaluate[string](checkScript,
		chromedp.EvalAwaitPromise))
	if err != nil {
		t.Fatal(err)
	}
	if want := "1|2|2"; got != want {
		t.Errorf("reads per check = %q, want %q", got, want)
	}
}
