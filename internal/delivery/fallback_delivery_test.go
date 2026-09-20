package delivery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestNotifyIncidentEventDeliveryProviderPropagatesActionAndDedup(
	t *testing.T,
) {
	fp := &fakeRecordingEventProvider{done: make(chan struct{})}
	am := *newTestManager()
	appendManagerEntries(&am, providerEntry{
		provider: fp,
		retry:    retryConfig{maxAttempts: 1},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := am.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer am.shutdown()

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
			ID:        "abc123",
		},
		Status: model.Status{
			Count:     1,
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
			Resources: map[string]bool{"pod-1": true},
		},
	}

	am.NotifyIncident(inc, model.ActionResolved, nil)
	select {
	case <-fp.done:
	case <-time.After(time.Second):
		t.Fatal("event provider was not called")
	}

	if fp.lastEvent == nil {
		t.Fatal("expected SendEvent to be called")
	}
	assert.Equal(t, "resolved", fp.lastEvent.Action)
	assert.Equal(t, "abc123", fp.lastEvent.DedupKey)
}

type fakeRecordingEventProvider struct {
	lastEvent *event.Event
	done      chan struct{}
}

func (p *fakeRecordingEventProvider) SendMessage(
	_ context.Context,
	msg string,
) error {
	return nil
}

func (p *fakeRecordingEventProvider) SendEvent(
	_ context.Context,
	evt *event.Event,
) error {
	p.lastEvent = evt
	if p.done != nil {
		close(p.done)
	}
	return nil
}

func (p *fakeRecordingEventProvider) Name() string       { return "Recording" }
func (p *fakeRecordingEventProvider) UsesEventDelivery() {}

// A late NotifyIncident after shutdown must be a no-op, not a send-on-closed
// panic: shutdown closes provider channels and fanOut must not send on them.
func TestNotifyIncidentAfterShutdownIsNoop(t *testing.T) {
	am := newTestManager()
	setManagerEntries(am, []providerEntry{{
		provider: &fakeProvider{},
		retry:    retryConfig{maxAttempts: 1},
		ch:       make(chan deliverJob, channelCap),
	}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	am.Start(ctx)
	am.shutdown()

	am.NotifyIncident(&model.Incident{
		Subject: model.Subject{
			Key:    "k",
			Name:   "n",
			Reason: "OOMKilled",
		},
	}, model.ActionCreate, nil)
}

func TestManagerCanRestartAfterShutdown(t *testing.T) {
	am := newTestManager()
	setManagerEntries(am, []providerEntry{{
		provider: &fakeProvider{},
		retry:    retryConfig{maxAttempts: 1},
		ch:       make(chan deliverJob, channelCap),
	}})

	ctx1, cancel1 := context.WithCancel(context.Background())
	am.Start(ctx1)
	am.shutdown()
	cancel1()

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	am.Start(ctx2)
	am.NotifyIncident(&model.Incident{
		Subject: model.Subject{Key: "restart", Name: "n", Reason: "Error"},
	}, model.ActionCreate, nil)
	am.shutdown()
}
