package scorecard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/audit"
)

func TestScoreUnchangedUpdatesCountByContentHash(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:   now,
			Problem:     "ns:key",
			Reason:      "Test",
			Action:      audit.ActionUpdate,
			ContentHash: "abc123",
		},
		{
			Timestamp:   now.Add(time.Second),
			Problem:     "ns:key",
			Reason:      "Test",
			Action:      audit.ActionUpdate,
			ContentHash: "abc123",
		},
		{
			Timestamp:   now.Add(2 * time.Second),
			Problem:     "ns:key",
			Reason:      "Test",
			Action:      audit.ActionUpdate,
			ContentHash: "def456",
		},
	}
	report := Score(entries)
	assert.Equal(t, 3, report.Updates)
	assert.Equal(t, 1, report.UnchangedUpdates)
}

func TestScoreRecreatedAfterResolve(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp: now,
			Problem:   "ns:key",
			Reason:    "Test",
			Action:    audit.ActionCreate,
		},
		{
			Timestamp: now.Add(time.Second),
			Problem:   "ns:key",
			Reason:    "Test",
			Action:    audit.ActionResolved,
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Problem:   "ns:key",
			Reason:    "Test",
			Action:    audit.ActionCreate,
		},
	}
	report := Score(entries)
	assert.Equal(t, 1, report.Recreated)
}

func TestScoreRepeatedResolves(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp: now,
			Problem:   "ns:key",
			Reason:    "Test",
			Action:    audit.ActionResolved,
		},
		{
			Timestamp: now.Add(time.Second),
			Problem:   "ns:key",
			Reason:    "Test",
			Action:    audit.ActionResolved,
		},
	}
	report := Score(entries)
	assert.Equal(t, 1, report.RepeatedResolves)
}

func TestScoreP95Percentile(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{}
	for i := 0; i < 100; i++ {
		key := "ns:k" + string(rune('0'+i%10))
		entries = append(entries, audit.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Problem:   key,
			Reason:    "Test",
			Action:    audit.ActionCreate,
		})
	}
	report := Score(entries)
	assert.Equal(t, 10, report.Problems)
	assert.Greater(t, report.PerProblemP95, 0)
}

func TestScoreGroupedProblems(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:     now,
			Problem:       "ns:key1",
			Reason:        "Test",
			Action:        audit.ActionCreate,
			AffectedCount: 3,
		},
		{
			Timestamp:     now.Add(time.Second),
			Problem:       "ns:key2",
			Reason:        "Test",
			Action:        audit.ActionCreate,
			AffectedCount: 5,
		},
	}
	report := Score(entries)
	assert.Equal(t, 2, report.Grouped)
}

func TestScoreUnknownCause(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:  now,
			Problem:    "ns:key1",
			Reason:     "Test",
			Action:     audit.ActionCreate,
			CauseState: "unknown",
		},
		{
			Timestamp:  now.Add(time.Second),
			Problem:    "ns:key2",
			Reason:     "Test",
			Action:     audit.ActionCreate,
			CauseState: "pod",
		},
		{
			Timestamp: now.Add(2 * time.Second),
			Problem:   "ns:key3",
			Reason:    "Test",
			Action:    audit.ActionCreate,
		},
	}
	report := Score(entries)
	assert.Equal(t, 3, report.Notifications)
	assert.Equal(t, 2, report.UnknownCause)
}

func TestScoreResolvedNotCountedAsUnknown(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:  now,
			Problem:    "ns:key",
			Reason:     "Test",
			Action:     audit.ActionResolved,
			CauseState: "",
		},
	}
	report := Score(entries)
	assert.Equal(t, 0, report.UnknownCause)
}

func TestScoreCauseStates(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entry := func(key, state string, action audit.Action) audit.Entry {
		return audit.Entry{
			Timestamp: now, Problem: key, Action: action, CauseState: state,
		}
	}
	report := Score([]audit.Entry{
		entry("a", audit.CauseKnown, audit.ActionCreate),
		entry("b", audit.CauseSelf, audit.ActionCreate),
		entry("c", audit.CauseUnknown, audit.ActionCreate),
		entry("d", "", audit.ActionCreate),
		entry("e", audit.CauseUnknown, audit.ActionResolved),
	})
	assert.Equal(t, 1, report.CircularCause)
	assert.Equal(t, 2, report.UnknownCause)
}
