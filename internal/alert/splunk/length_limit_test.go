package splunk

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The message and output of an over-long incident are each cut to
// splunkFieldLimit, so the collector accepts the event.
func TestSplunkCutsOverlongIncident(t *testing.T) {
	c, rec := newTestSplunk(t)
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	ev := rec.Last(t).JSON(t)["event"].(map[string]any)
	providertest.AssertWithin(t, "message", ev["message"].(string),
		splunkFieldLimit)
	var lines []string
	for _, line := range ev["output"].([]any) {
		lines = append(lines, line.(string))
	}
	providertest.AssertWithin(t, "output", strings.Join(lines, ""),
		splunkFieldLimit)
}

func TestSplunkBoundLines(t *testing.T) {
	got := boundLines([]string{"aaaa", "bbbbbbbbbb", "cc"}, 9)
	if len(got) != 2 || got[0] != "aaaa" || got[1] != "bb…" {
		t.Fatalf("boundLines = %q", got)
	}
	if got := boundLines([]string{"aaaa"}, 4); len(got) != 1 {
		t.Fatalf("a line that fits exactly is kept: %q", got)
	}
}
