package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
)

// stoppedAtCap is a resolve at the still-broken cap with the given
// reason tail.
func stoppedAtCap(tail string) incident.Decision {
	p := podCrash(incident.Notify)
	p.State, p.Resolved = incident.Resolved, writerNow
	return incident.Decision{Action: incident.Resolve, Incident: p,
		Reason: incident.Reason(incident.StoppedTrackingPrefix +
			"2h; coverage check continues" + tail)}
}

// The lead of a resolve at the cap names the condition that held, and
// the message says plainly that kwatch stopped and it is not healthy.
func TestStoppedTrackingLeadFollowsTheCondition(t *testing.T) {
	for tail, want := range map[string]string{
		"": "has not recovered.",
		incident.StillCrashingNote: "still has a container that keeps " +
			"crashing.",
	} {
		note := Writer{}.Write(stoppedAtCap(tail), writerNow).Note
		if !strings.Contains(note, want) {
			t.Errorf("tail %q: want %q in %q", tail, want, note)
		}
		if !strings.Contains(note, "kwatch stopped tracking this after "+
			"2h; it is still not healthy, so check it yourself.") {
			t.Errorf("tail %q: note = %q", tail, note)
		}
	}
	note := Writer{}.Write(stoppedAtCap(incident.StillCrashingNote),
		writerNow).Note
	if strings.Contains(note, "short of replicas") {
		t.Errorf("a crash loop at full replicas is not short: %q", note)
	}
}

// A workload subject held by its replicas is said to be short of them.
func TestStoppedTrackingWorkloadIsShortOfReplicas(t *testing.T) {
	d := stoppedAtCap("")
	d.Incident.Root = workload("payments")
	note := Writer{}.Write(d, writerNow).Note
	if !strings.Contains(note, "is still short of replicas.") {
		t.Errorf("note = %q", note)
	}
}
