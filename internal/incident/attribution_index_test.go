package incident

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestOwnerIncidentForRequiresIndexedLiveIncident(t *testing.T) {
	engine := newTestEngine()
	eventValue := event.Event{Resource: "pod", Namespace: "ns"}

	engine.mu.Lock()
	owner, ok := engine.ownerIncidentFor(eventValue, "api", "pod")
	engine.mu.Unlock()
	require.False(t, ok)
	require.Nil(t, owner)
}

func TestOwnerIncidentForUsesIndexedOwnerReference(t *testing.T) {
	engine := newTestEngine()
	ownerIncident := &model.Incident{
		Subject: model.Subject{
			Key:       BuildKey("ns", OwnerPath("ns", "api"), "RolloutStuck", ""),
			Resource:  "deployment",
			Namespace: "ns",
			Name:      "ns/api",
			Object: model.ObjectRef{
				Kind: "deployment", Namespace: "ns", Name: "api",
			},
		},
		Status: model.Status{State: model.StateActive},
	}
	eventValue := event.Event{Resource: "pod", Namespace: "ns"}

	engine.mu.Lock()
	engine.state[ownerIncident.Key] = ownerIncident
	engine.indexIncident(ownerIncident)
	got, ok := engine.ownerIncidentFor(eventValue, "api", "pod")
	engine.mu.Unlock()

	require.True(t, ok)
	require.Same(t, ownerIncident, got)
}
