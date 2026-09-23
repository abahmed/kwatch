package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

type structuredRecordingProvider struct {
	notifications chan *message.Notification
}

func (p *structuredRecordingProvider) Name() string { return "structured" }

func (p *structuredRecordingProvider) SendMessage(
	context.Context,
	string,
) error {
	return nil
}

func (p *structuredRecordingProvider) SendEvent(
	context.Context,
	*event.Event,
) error {
	return nil
}

func (p *structuredRecordingProvider) SendNotification(
	_ context.Context,
	notification *message.Notification,
) error {
	p.notifications <- notification
	return nil
}

func TestDispatchIncidentUsesProviderNeutralNotification(t *testing.T) {
	provider := &structuredRecordingProvider{
		notifications: make(chan *message.Notification, 1),
	}
	manager := newTestManager()
	manager.clusterName = "production"
	entry := &providerEntry{
		provider: provider,
		retry:    retryConfig{maxAttempts: 1},
	}
	incident := &model.Incident{
		Subject: model.Subject{
			ID:        "incident-1",
			Name:      "checkout",
			Namespace: "payments",
			Resource:  "deployment",
			Reason:    "DeploymentUnavailable",
		},
		Status: model.Status{
			FirstSeen: time.Now().Add(-time.Minute),
			LastSeen:  time.Now(),
		},
		Delivery: model.Delivery{Revision: 3},
	}

	err := manager.dispatchIncident(
		context.Background(), entry,
		incidentJob(incident, model.ActionCreate, nil),
		deliverOpts{retry: entry.retry},
	)
	require.NoError(t, err)
	select {
	case notification := <-provider.notifications:
		require.Equal(t, "incident-1:3:create", notification.DeliveryID)
		require.Equal(t, "production", notification.Summary.Location.Cluster)
		require.Equal(t, "payments", notification.Summary.Location.Namespace)
	case <-time.After(time.Second):
		t.Fatal("structured provider did not receive notification")
	}
}
