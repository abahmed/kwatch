package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

const refusedRedisLog = "FATAL: dial tcp 10.96.0.42:6379: connect: " +
	"connection refused"

// recreatedDecision is checkout-api deleted and created again at once,
// crashing with a log line that says why, while kwatch reports blind
// spots.
func recreatedDecision(now time.Time, logLine string) incident.Decision {
	root := inventory.CoreID("deployment", "default", "checkout-api")
	pod := inventory.CoreID("pod", "default", "checkout-api-1")
	f := detection.Finding{
		Entity: pod, Reason: "CrashLoopBackOff",
		Severity: detection.Critical, Since: now.Add(-time.Minute),
		Summary: "Crash looping", Evidence: []detection.Evidence{
			{Label: detection.EvidenceError, Value: logLine}},
	}
	deletedAt := now.Add(-2 * time.Minute)
	return incident.Decision{Action: incident.Announce,
		Incident: incident.Incident{
			ID: "inc-1", Root: root, Tier: incident.Notify,
			State: incident.Open, Opened: now.Add(-time.Minute),
			Members: map[detection.Key]detection.Finding{f.Key(): f},
			Unverified: []string{"config maps in default", "nodes",
				"service accounts in default"},
		},
		Facts: incident.Facts{Changes: []inventory.Change{
			{Entity: root, At: deletedAt.Add(time.Second), Created: true},
			{Entity: root, At: deletedAt, Deleted: true},
		}},
	}
}

func joined(lines []string) string { return strings.Join(lines, "\n") }

// The deletion of the workload's own earlier incarnation is not a
// change before it broke.
func TestRecreatedWorkloadOmitsOwnDeletion(t *testing.T) {
	now := time.Date(2026, 10, 27, 23, 14, 0, 0, time.UTC)
	msg := Writer{}.Write(recreatedDecision(now, refusedRedisLog), now)

	assert.NotContains(t, joined(msg.Lines), "was deleted")
}

// A workload that never worked and whose crash log says why needs no
// caveat about what kwatch cannot see.
func TestNeverWorkedWithClearLogOmitsUnverified(t *testing.T) {
	now := time.Date(2026, 10, 27, 23, 14, 0, 0, time.UTC)
	msg := Writer{}.Write(recreatedDecision(now, refusedRedisLog), now)

	assert.NotContains(t, joined(msg.Lines), "can't see")
}

// Without a log line that says why, the caveat stays.
func TestNeverWorkedWithoutClearLogKeepsUnverified(t *testing.T) {
	now := time.Date(2026, 10, 27, 23, 14, 0, 0, time.UTC)
	msg := Writer{}.Write(recreatedDecision(now, "exit status 1"), now)

	assert.Contains(t, joined(msg.Lines), "can't see")
}

// The deletion of another object stays, as does a deletion that was
// not followed by a new workload of the same name.
func TestWithoutOwnPredecessorKeepsOtherDeletions(t *testing.T) {
	now := time.Date(2026, 10, 27, 23, 14, 0, 0, time.UTC)
	d := recreatedDecision(now, refusedRedisLog)
	f := gatherFacts(d, now, nil)
	other := inventory.CoreID("configmap", "default", "flags")
	changes := []inventory.Change{
		{Entity: other, At: now.Add(-3 * time.Minute), Deleted: true},
		{Entity: d.Incident.Root, At: now.Add(-time.Minute),
			Deleted: true},
	}

	assert.Equal(t, changes, withoutOwnPredecessor(f, changes))
}
