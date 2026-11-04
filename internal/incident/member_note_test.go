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
