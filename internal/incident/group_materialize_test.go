package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestGroupIncidentDoesNotUseFirstMemberEvidence(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	engine := newTestEngine(Config{
		Window:                   10 * time.Minute,
		SmartGroupingWindow:      60 * time.Second,
		NamespaceFanOutThreshold: 2,
	})
	engine.now = mockClock(now)
	var group *model.Incident
	var delivered []model.IncidentKey
	engine.config.LifecycleHook = func(
		incident *model.Incident, _ model.IncidentAction,
	) {
		delivered = append(delivered, incident.Key)
		if IsGroupKey(incident.Key) {
			group = incident.Clone()
		}
	}
	engine.processEvent(event.Event{
		PodName: "first", Namespace: "ns", Reason: "CrashLoopBackOff",
		Logs: "shared failure evidence", ContainerName: "app",
	}, "first-owner", nil)
	engine.processEvent(event.Event{
		PodName: "second", Namespace: "ns", Reason: "CrashLoopBackOff",
		Logs: "shared failure evidence", ContainerName: "worker",
	}, "second-owner", nil)
	engine.processEvent(event.Event{
		PodName: "third", Namespace: "ns", Reason: "CrashLoopBackOff",
		Logs: "shared failure evidence", ContainerName: "sidecar",
	}, "third-owner", nil)

	engine.now = mockClock(now.Add(61 * time.Second))
	engine.checkLifecycle()

	require.NotNilf(t, group, "delivered: %v", delivered)
	require.Empty(t, group.Logs)
	require.Empty(t, group.ContainerName)
	require.Len(t, group.AffectedMembers, 3)
	require.Equal(t, "first", group.AffectedMembers[0].Pod)
	require.Equal(t, "second", group.AffectedMembers[1].Pod)
}
