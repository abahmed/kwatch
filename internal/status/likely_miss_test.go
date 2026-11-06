package status

import (
	"strings"
	"testing"
	"time"
)

func TestTextListsLikelyMissesWithTheirExportWindow(t *testing.T) {
	r := checkedReport()
	at := time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC)
	r.LikelyMisses = []LikelyMiss{{ID: "inc-1", Rule: "config-missing",
		Root: "configmap/shop/settings", Other: "pod/shop/api-1",
		Change: "image", Opened: at, Resolved: at.Add(15 * time.Minute),
		ExportSince: at.Add(-10 * time.Minute),
		ExportUntil: at.Add(25 * time.Minute)}}
	text := r.Text()
	for _, want := range []string{
		"Likely wrong causes (last 7 days)",
		"  inc-1: named configmap/shop/settings (config-missing); " +
			"pod/shop/api-1 changed (image)",
		"    kwatch replay-export --since 2026-01-10T08:50:00Z " +
			"--until 2026-01-10T09:25:00Z",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}
