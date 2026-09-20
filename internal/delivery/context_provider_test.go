package delivery

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/event"
)

type contextRecordingProvider struct {
	eventContext   context.Context
	messageContext context.Context
}

type contextKey struct{}

func (p *contextRecordingProvider) Name() string {
	return "ContextRecording"
}

func (p *contextRecordingProvider) SendEvent(
	ctx context.Context,
	_ *event.Event,
) error {
	p.eventContext = ctx
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

	if err := provider.SendEvent(ctx, &event.Event{}); err != nil {
		t.Fatalf("SendEvent() returned error: %v", err)
	}
	if err := provider.SendMessage(ctx, "message"); err != nil {
		t.Fatalf("SendMessage() returned error: %v", err)
	}
	if provider.eventContext != ctx || provider.messageContext != ctx {
		t.Fatal("provider did not receive the delivery context")
	}
}
