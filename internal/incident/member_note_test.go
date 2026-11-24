package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestMemberNoteRecordsProbeCountAndMessage(t *testing.T) {
	id := inventory.CoreID("container", "web", "api-1/app")
	probe := detection.Finding{Entity: id,
		Summary: "Container keeps failing its readiness probe",
		Evidence: []detection.Evidence{
			{Label: "probe", Value: "Readiness probe failed: Get " +
				"\"http://10.0.0.5:80/ready\": connect: connection refused"},
			{Label: "failures", Value: "23"}}}
	want := "Container keeps failing its readiness probe: 23 failures, " +
		"last: Readiness probe failed: Get \"http://10.0.0.5:80/ready\": " +
		"connect: connection refused (container web/api-1/app)"
	if got := memberNote(probe); got != want {
		t.Fatalf("note = %q\nwant %q", got, want)
	}
	plain := detection.Finding{Entity: id, Summary: "Container keeps " +
		"restarting"}
	if got := memberNote(plain); got != "Container keeps restarting "+
		"(container web/api-1/app)" {
		t.Fatalf("note = %q", got)
	}
}

// A restored incident had pod a failing with CrashLoopBackOff and pod b
// with OOMKilled. Pod a coming back with OOMKilled is a new member, not
// one it had: the object and reason are compared as one pair.
func TestRejoinedComparesObjectAndReasonTogether(t *testing.T) {
	a := inventory.CoreID("pod", "web", "a")
	b := inventory.CoreID("pod", "web", "b")
	p := &Incident{Members: map[detection.Key]detection.Finding{
		{Entity: a, Reason: "CrashLoopBackOff"}: {},
		{Entity: b, Reason: "OOMKilled"}:        {},
	}}
	record := Record{MemberKeys: savedKeys(p),
		Members: memberEntities(p), Reasons: memberReasons(p)}
	restored := &Incident{restored: true,
		formerKeys: formerKeys(record)}

	if !rejoined(restored, detection.Finding{Entity: a,
		Reason: "CrashLoopBackOff"}) {
		t.Fatal("the member it had is not noted again")
	}
	if rejoined(restored, detection.Finding{Entity: a,
		Reason: "OOMKilled"}) {
		t.Fatal("pod a with OOMKilled was never a member")
	}
	old := Record{Members: record.Members, Reasons: record.Reasons}
	restored.formerKeys = formerKeys(old)
	if !rejoined(restored, detection.Finding{Entity: b,
		Reason: "OOMKilled"}) {
		t.Fatal("an older record still knows its members")
	}
}
