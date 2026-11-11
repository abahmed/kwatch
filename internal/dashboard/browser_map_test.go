//go:build browser

package dashboard

import (
	"context"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/abahmed/kwatch/internal/inventory"
)

// browserMapModel is a small namespace for an issue's "How it connects"
// picture in the browser check: an Ingress, its Service, a Deployment
// with a pod, and a Service of another namespace the pod calls
// (mapShop).
func browserMapModel(t *testing.T) inventory.HistoryReader {
	tm, _ := mapShop(t)
	return tm.m
}

// mapScript reports what is wrong with the picture on the Impact tab of
// an issue whose object kwatch knows connections of, or "".
const mapScript = `(() => {
  const fold = document.querySelector('[data-fold="map"]');
  if (!fold || fold.hidden) return "the How it connects fold is hidden";
  const boxes = fold.querySelectorAll(".map-node").length;
  if (boxes < 3) return "the picture draws " + boxes + " objects";
  if (!fold.querySelector(".map-self")) return "no object is the issue's";
  if (!fold.querySelector(".map-list li")) return "the list is empty";
  return "";
})()`

// checkIssueMap opens an issue's Impact tab at a desktop size and checks
// the picture of what the issue touches.
func checkIssueMap(t *testing.T, ctx context.Context, base string) {
	t.Helper()
	err := chromedp.Do(ctx,
		chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(base+"/issues/inc-001#impact"),
		chromedp.WaitVisible(chromedp.CSS(`[data-fold="map"] svg`)),
	)
	if err != nil {
		t.Fatalf("issue map: %v", err)
	}
	got, err := chromedp.Run(ctx, chromedp.Evaluate[string](mapScript))
	if err != nil {
		t.Fatalf("issue map: %v", err)
	}
	if got != "" {
		t.Errorf("issue map: %s", got)
	}
}
