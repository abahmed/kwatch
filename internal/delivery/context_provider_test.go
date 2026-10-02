package delivery

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/notification"
)

type contextRecordingProvider struct {
	incidentContext context.Context
	messageContext  context.Context
}

type contextKey struct{}

func (p *contextRecordingProvider) Name() string {
	return "ContextRecording"
}

func (p *contextRecordingProvider) SendIncident(
	ctx context.Context,
	_ notification.Message,
) error {
	p.incidentContext = ctx
	return nil
}

func (p *contextRecordingProvider) SendMessage(
	ctx context.Context,
	_ string,
) error {
	p.messageContext = ctx
	return nil
}

func TestProviderReceivesContext(t *testing.T) {
	provider := &contextRecordingProvider{}
	ctx := context.WithValue(context.Background(), contextKey{}, "value")

	if err := provider.SendIncident(
		ctx, notification.Message{},
	); err != nil {
		t.Fatalf("SendIncident() returned error: %v", err)
	}
	if err := provider.SendMessage(ctx, "message"); err != nil {
		t.Fatalf("SendMessage() returned error: %v", err)
	}
	if provider.incidentContext != ctx || provider.messageContext != ctx {
		t.Fatal("provider did not receive the delivery context")
	}
}
