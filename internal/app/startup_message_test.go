package app

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notification"
)

// The startup message is one plain sentence with one status marker and
// no links or shortcodes.
func TestStartupSentence(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		reason   string
		downtime time.Duration
		reset    bool
		want     string
	}{
		"first start": {"", 0, false, "🟡 kwatch v1.2.0 started."},
		"after a rollout": {"deployment_rollout", 0, false,
			"🟡 kwatch v1.2.0 started again after a kwatch rollout."},
		"after a clean stop": {"graceful_shutdown", 0, false,
			"🟡 kwatch v1.2.0 started."},
		"after a crash": {"internal_failure", 0, false,
			"🟠 kwatch v1.2.0 started again after an unexpected " +
				"kwatch stop."},
		"after a gap": {"", 12 * time.Minute, false,
			"🟠 kwatch v1.2.0 started; nothing was monitored from " +
				"11:48 to 12:00 UTC (12m), so anything that broke " +
				"then went unreported. Pro adds a gap report: what " +
				"changed and what crashed meanwhile. Free during early " +
				"access."},
		"after a node disruption with a gap": {"node_disruption",
			time.Hour, false,
			"🟠 kwatch v1.2.0 started again after a node disruption; " +
				"nothing was monitored from 11:00 to 12:00 UTC (1h), " +
				"so anything that broke then went unreported. Pro adds " +
				"a gap report: what changed and what crashed meanwhile. " +
				"Free during early access."},
		"after a gap across days": {"", 30 * time.Hour, false,
			"🟠 kwatch v1.2.0 started; nothing was monitored from " +
				"Sep 30 06:00 to Oct 1 12:00 UTC (30h), so anything " +
				"that broke then went unreported. Pro adds a gap " +
				"report: what changed and what crashed meanwhile. Free " +
				"during early access."},
		"after a state reset": {"", 0, true,
			"🟠 kwatch v1.2.0 started with fresh state: earlier " +
				"incident history was reset, so open problems will be " +
				"announced again."},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := startupSentence(startupFacts{version: "v1.2.0",
				restartReason: c.reason, now: now, downtime: c.downtime,
				stateReset: c.reset})
			if got != c.want {
				t.Fatalf("message =\n%q\nwant\n%q", got, c.want)
			}
			assertPlainSentence(t, got)
		})
	}
}

// assertPlainSentence checks the notice rules: exactly one leading
// status marker, one sentence, no line breaks, links or shortcodes.
func assertPlainSentence(t *testing.T, text string) {
	t.Helper()
	markers := 0
	for _, m := range notification.Markers() {
		markers += strings.Count(text, m)
	}
	n := notification.Notice(text)
	if markers != 1 || n.Marker == "" ||
		!strings.HasPrefix(text, n.Marker+" ") {
		t.Fatalf("want one leading marker: %q", text)
	}
	for _, bad := range []string{"\n", "http", "<", ":tada:", "⚠️"} {
		if strings.Contains(text, bad) {
			t.Fatalf("message contains %q: %q", bad, text)
		}
	}
	// The Community Pro hint is the one sentence allowed after it.
	text = strings.Replace(text, "."+proGapHint, "", 1)
	if strings.Count(strings.TrimSuffix(text, "."), ". ") > 0 ||
		!strings.HasSuffix(text, ".") {
		t.Fatalf("want one sentence: %q", text)
	}
}
