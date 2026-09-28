package scorecard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/audit"
)

func TestScoreUnchangedUpdatesCountByRenderingHash(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:     now,
			IncidentKey:   "ns:key",
			Reason:        "Test",
			Action:        audit.ActionUpdate,
			RenderingHash: "abc123",
		},
		{
			Timestamp:     now.Add(time.Second),
			IncidentKey:   "ns:key",
			Reason:        "Test",
			Action:        audit.ActionUpdate,
			RenderingHash: "abc123",
		},
		{
			Timestamp:     now.Add(2 * time.Second),
			IncidentKey:   "ns:key",
			Reason:        "Test",
			Action:        audit.ActionUpdate,
			RenderingHash: "def456",
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
			Timestamp:   now,
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionCreate,
		},
		{
			Timestamp:   now.Add(time.Second),
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionResolved,
		},
		{
			Timestamp:   now.Add(2 * time.Second),
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionCreate,
		},
	}
	report := Score(entries)
	assert.Equal(t, 1, report.Recreated)
}

func TestScoreRepeatedResolves(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:   now,
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionResolved,
		},
		{
			Timestamp:   now.Add(time.Second),
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionResolved,
		},
	}
	report := Score(entries)
	assert.Equal(t, 1, report.RepeatedResolves)
}

func TestScoreCircularCauseKindAware(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		reason      string
		rootCause   string
		objectName  string
		expectCount int
	}{
		{
			name:        "deployment circular",
			reason:      "DeploymentUnavailable",
			rootCause:   "deployment ns/app",
			objectName:  "ns/app",
			expectCount: 1,
		},
		{
			name:        "service with deployment cause not circular",
			reason:      "ServiceNoEndpoints",
			rootCause:   "deployment ns/app",
			objectName:  "ns/api",
			expectCount: 0,
		},
		{
			name:        "hpa cause mapped correctly",
			reason:      "FailedGetResourceMetric",
			rootCause:   "horizontalpodautoscaler ns/scaler",
			objectName:  "ns/scaler",
			expectCount: 1,
		},
		{
			name:        "pod crash loop",
			reason:      "CrashLoopBackOff",
			rootCause:   "pod ns/pod1",
			objectName:  "ns/pod1",
			expectCount: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entries := []audit.Entry{
				{
					Timestamp:   now,
					IncidentKey: "ns:key",
					Reason:      test.reason,
					Action:      audit.ActionCreate,
					Name:        test.objectName,
					RootCause:   test.rootCause,
				},
			}
			report := Score(entries)
			assert.Equal(t, test.expectCount, report.CircularCause)
		})
	}
}

func TestScoreP95Percentile(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{}
	for i := 0; i < 100; i++ {
		key := "ns:k" + string(rune('0'+i%10))
		entries = append(entries, audit.Entry{
			Timestamp:   now.Add(time.Duration(i) * time.Second),
			IncidentKey: key,
			Reason:      "Test",
			Action:      audit.ActionCreate,
		})
	}
	report := Score(entries)
	assert.Equal(t, 10, report.Incidents)
	assert.Greater(t, report.PerIncidentP95, 0)
}

func TestScoreSkipsNonNotifiedEntries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:   now,
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionSkip,
		},
		{
			Timestamp:   now.Add(time.Second),
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionCreate,
			Decision:    "nonotify",
		},
		{
			Timestamp:   now.Add(2 * time.Second),
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionCreate,
			Decision:    "notify",
		},
	}
	report := Score(entries)
	assert.Equal(t, 1, report.Notifications)
}

func TestScoreGroupedIncidents(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{
			Timestamp:   now,
			IncidentKey: "ns:key1",
			Reason:      "Test",
			Action:      audit.ActionCreate,
			GroupKey:    "group",
		},
		{
			Timestamp:     now.Add(time.Second),
			IncidentKey:   "ns:key2",
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
			Timestamp:   now,
			IncidentKey: "ns:key1",
			Reason:      "Test",
			Action:      audit.ActionCreate,
			CauseState:  "unknown",
		},
		{
			Timestamp:   now.Add(time.Second),
			IncidentKey: "ns:key2",
			Reason:      "Test",
			Action:      audit.ActionCreate,
			CauseState:  "pod",
		},
		{
			Timestamp:   now.Add(2 * time.Second),
			IncidentKey: "ns:key3",
			Reason:      "Test",
			Action:      audit.ActionCreate,
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
			Timestamp:   now,
			IncidentKey: "ns:key",
			Reason:      "Test",
			Action:      audit.ActionResolved,
			CauseState:  "",
		},
	}
	report := Score(entries)
	assert.Equal(t, 0, report.UnknownCause)
}
