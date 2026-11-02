package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// The resolve of an incident only the startup summary announced has no
// thread of its own in chat: it names that summary as its carrier, so
// the send log says it stands alone, and it speaks of what the summary
// named, which the incident remembers as its subject.
func TestEngineResolveOfSummaryListedIncidentSpeaksAsAnnounced(
	t *testing.T,
) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h, recover := coldHarness(t, start)
	collect := h.engine.announcer.collect
	if !h.runUntil(func() bool { return collect.Startup.Summary.Complete },
		start.Add(5*time.Minute), 10*time.Second) {
		t.Fatalf("startup summary never completed:\n%s", joinTitles(h))
	}
	id := collect.Startup.Summary.Incidents[0]
	if got := collect.ListedBy(id); got != "startup summary" {
		t.Fatalf("listed by %q", got)
	}
	for _, p := range h.engine.deps.Incidents.Incidents() {
		if p.ID == id && p.Subject.IsZero() {
			t.Fatal("the summary's subject was not recorded")
		}
	}

	recover()
	h.runUntil(func() bool { return resolvedBy(h, id) },
		h.now.Add(10*time.Minute), 10*time.Second)
	for i, d := range h.decisions {
		if d.Action != incident.Resolve || h.messages[i].Key != id {
			continue
		}
		if !d.AsAnnounced {
			t.Error("the resolve does not speak as announced")
		}
		if got := h.messages[i].ChatCarrier; got != "startup summary" {
			t.Errorf("chat carrier = %q", got)
		}
		return
	}
	t.Fatalf("no resolve:\n%s", joinTitles(h))
}
