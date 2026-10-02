package scorecard

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/audit"
)

func TestGatesCompareValuesWithTargets(t *testing.T) {
	gates := []Gate{
		AtMost("messages", 3, 3, ""),
		AtMost("rate", 20.5, 20, "/h"),
		AtLeast("correct", 91.25, 90, "%"),
		AtLeast("calibrated", 0, 1, ""),
	}
	assert.True(t, gates[0].Pass)
	assert.False(t, gates[1].Pass)
	assert.Equal(t, "20.5/h", gates[1].Value)
	assert.Equal(t, "<= 20/h", gates[1].Target)
	assert.True(t, gates[2].Pass)
	failed := Failed(gates)
	assert.Len(t, failed, 2)
	table := GateTable(gates)
	assert.True(t, strings.HasPrefix(table, "| Metric |"))
	assert.Contains(t, table, "| rate | <= 20/h | 20.5/h | fail |")
	assert.Contains(t, table, "| messages | <= 3 | 3 | pass |")
}

func TestGoalsFlagProductionLimits(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	violations := Goals().Violations(Score(scoreEntries(start)))
	joined := strings.Join(violations, "\n")
	assert.Contains(t, joined, "re-created incidents %")
	assert.NotContains(t, joined, "most messages for one incident")
	assert.Empty(t, NoThresholds().Violations(Score(scoreEntries(start))))
}

// scoreEntries is one incident that resolves and comes back, and one
// incident announced once, all inside one hour.
func scoreEntries(start time.Time) []audit.Entry {
	at := func(minutes int) time.Time {
		return start.Add(time.Duration(minutes) * time.Minute)
	}
	return []audit.Entry{
		{Timestamp: at(0), Incident: "a", Action: audit.ActionCreate},
		{Timestamp: at(10), Incident: "a", Action: audit.ActionResolved},
		{Timestamp: at(20), Incident: "a", Action: audit.ActionCreate},
		{Timestamp: at(90), Incident: "b", Action: audit.ActionCreate},
	}
}
