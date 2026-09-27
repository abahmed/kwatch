package event

import "testing"

func TestEventLifecycleHelpers(t *testing.T) {
	notice := &Event{PodName: "kwatch started\nmore", Reason: "notify"}
	if !notice.IsNotice() || notice.AlertKey() != "kwatch-notice" ||
		notice.AlertTitle(0) != "kwatch started" {
		t.Fatalf("notice helpers wrong: %+v", notice)
	}
	incident := &Event{
		DedupKey: "abc", Action: "resolved",
		Narrative: "Recovered: api is healthy\ndetails",
	}
	if !incident.IsResolve() || incident.IsNotice() ||
		incident.AlertKey() != "kwatch-abc" {
		t.Fatalf("incident helpers wrong: %+v", incident)
	}
	if got := incident.AlertTitle(12); got != "Recovered…" {
		t.Fatalf("AlertTitle(12) = %q", got)
	}
	if incident.AlertBody("c") != "Recovered: api is healthy\ndetails" {
		t.Fatalf("AlertBody = %q", incident.AlertBody("c"))
	}
}
