package alert

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestFallbackResolve(t *testing.T) {
	am := AlertManager{}
	am.Init(map[string]map[string]interface{}{
		"slack": {
			"webhook":  "test",
			"fallback": "pagerduty",
		},
		"pagerduty": {
			"integrationKey": "test",
		},
	}, &config.App{ClusterName: "dev"})

	var slackEntry, pagerEntry *providerEntry
	for i := range am.entries {
		switch am.entries[i].provider.Name() {
		case "Slack":
			slackEntry = &am.entries[i]
		case "PagerDuty":
			pagerEntry = &am.entries[i]
		}
	}
	if slackEntry == nil {
		t.Fatal("Slack entry not found")
	}
	if pagerEntry == nil {
		t.Fatal("PagerDuty entry not found")
	}
	if slackEntry.fallback != pagerEntry {
		t.Errorf("expected slack fallback to point to pagerduty entry")
	}
	if pagerEntry.fallback != nil {
		t.Errorf(
			"expected pagerduty to have no fallback, got %v",
			pagerEntry.fallback,
		)
	}
}

func TestFallbackResolveUnknown(t *testing.T) {
	am := AlertManager{}
	am.Init(map[string]map[string]interface{}{
		"slack": {
			"webhook":  "test",
			"fallback": "nonexistent",
		},
	}, &config.App{ClusterName: "dev"})

	if len(am.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(am.entries))
	}
	if am.entries[0].fallback != nil {
		t.Errorf("expected nil fallback for unknown target")
	}
}

// errorRecorderProvider records calls and optionally returns errors
type errorRecorderProvider struct {
	name      string
	msg       string
	err       error
	callCount int
}

func (p *errorRecorderProvider) SendMessage(msg string) error {
	p.msg = msg
	p.callCount++
	return p.err
}

func (p *errorRecorderProvider) SendEvent(
	evt *event.Event,
) error {
	return p.err
}

func (p *errorRecorderProvider) Name() string { return p.name }

func TestFallbackUsedOnExhaustion(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary", err: nil}
	fb := &errorRecorderProvider{name: "Fallback", err: nil}

	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{
		provider: primary,
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
		fallback: &providerEntry{provider: fb},
	})

	// primary succeeds — fallback should NOT be called
	am.Notify("test message")
	if primary.callCount != 1 {
		t.Errorf("expected 1 primary call, got %d", primary.callCount)
	}
	// Now make primary fail
	primary.err = errors.New("fail")
	primary.callCount = 0
	am.Notify("test message 2")
	if primary.callCount != 1 {
		t.Errorf(
			"expected 1 primary call on failure, got %d",
			primary.callCount,
		)
	}
	if fb.callCount != 1 {
		t.Errorf("expected 1 fallback call, got %d", fb.callCount)
	}
}

// The fallback message must respect the fallback provider's own maxBytes
// limit, not just the primary's.
func TestFallbackMessageTruncatedToFallbackMaxBytes(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary", err: nil}
	fb := &errorRecorderProvider{name: "Fallback", err: nil}

	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{
		provider: primary,
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
		fallback: &providerEntry{provider: fb, maxBytes: 64},
	})

	primary.err = errors.New("fail")
	am.Notify(strings.Repeat("x", 500))

	require.Equal(t, 1, fb.callCount)
	assert.LessOrEqual(
		t,
		len(fb.msg),
		64,
		"fallback message must respect the fallback provider's maxBytes",
	)
	assert.Contains(t, fb.msg, "(truncated)")
	assert.Contains(t, fb.msg, "fallback")
}

type eventFallbackProvider struct {
	eventCalls   int
	messageCalls int
}

func (p *eventFallbackProvider) Name() string { return "Event Fallback" }

func (p *eventFallbackProvider) SendEvent(*event.Event) error {
	p.eventCalls++
	return nil
}

func (p *eventFallbackProvider) SendMessage(string) error {
	p.messageCalls++
	return nil
}

func (p *eventFallbackProvider) UsesEventDelivery() {}

func TestIncidentFallbackUsesEventDeliveryInterface(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary", err: errors.New("fail")}
	fallback := &eventFallbackProvider{}
	am := AlertManager{entries: []providerEntry{{
		provider: primary,
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
		fallback: &providerEntry{
			provider: fallback,
			retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
		},
	}}}

	am.NotifyIncident(&model.Incident{
		Subject: model.Subject{Key: "ns:pod:Error", Reason: "Error"},
	}, model.ActionCreate, nil)

	assert.Equal(t, 1, fallback.eventCalls)
	assert.Zero(t, fallback.messageCalls)
}

func TestExtractRetryYAMLInt(t *testing.T) {
	// YAML v3 unmarshals integers as int, not float64.
	cfg := map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": 3,
			"delay":       "2s",
			"maxBackoff":  "10s",
		},
	}
	rc := extractRetry(cfg)
	assert.Equal(t, 3, rc.maxAttempts)
	assert.Equal(t, 2*time.Second, rc.delay)
	assert.Equal(t, 10*time.Second, rc.maxBackoff)
}

func TestExtractRetryJSONFloat(t *testing.T) {
	// JSON/CRD paths unmarshal numbers as float64.
	cfg := map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": float64(5),
		},
	}
	rc := extractRetry(cfg)
	assert.Equal(t, 5, rc.maxAttempts)
}

func TestExtractRetryClamps(t *testing.T) {
	cfg := map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": 0,
		},
	}
	rc := extractRetry(cfg)
	assert.Equal(t, 1, rc.maxAttempts)

	cfg = map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": 100,
		},
	}
	rc = extractRetry(cfg)
	assert.Equal(t, 20, rc.maxAttempts)
}

func TestExtractRetryDefaults(t *testing.T) {
	rc := extractRetry(map[string]interface{}{})
	assert.Equal(t, 3, rc.maxAttempts)
	assert.Equal(t, time.Second, rc.delay)
	assert.Equal(t, defaultMaxBackoff, rc.maxBackoff)
	assert.False(t, rc.jitterEnabled)
	assert.Equal(t, 0.25, rc.jitterFactor)
}

func TestSendWithRetryReturnsError(t *testing.T) {
	err := sendWithRetry(context.Background(), func() error {
		return errors.New("fail")
	}, retryConfig{maxAttempts: 1, delay: time.Millisecond}, "test")
	if err == nil {
		t.Fatal("expected error from sendWithRetry")
	}
}

func TestSendWithRetrySuccess(t *testing.T) {
	err := sendWithRetry(context.Background(), func() error {
		return nil
	}, retryConfig{maxAttempts: 3, delay: time.Millisecond}, "test")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestNotifyIncidentEventDeliveryProviderPropagatesActionAndDedup(
	t *testing.T,
) {
	fp := &fakeRecordingEventProvider{}
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{provider: fp, retry: retryConfig{maxAttempts: 1}},
	)

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

	if fp.lastEvent == nil {
		t.Fatal("expected SendEvent to be called")
	}
	assert.Equal(t, "resolved", fp.lastEvent.Action)
	assert.Equal(t, "abc123", fp.lastEvent.DedupKey)
}

type fakeRecordingEventProvider struct {
	lastEvent *event.Event
}

func (p *fakeRecordingEventProvider) SendMessage(
	msg string,
) error {
	return nil
}
func (p *fakeRecordingEventProvider) SendEvent(evt *event.Event) error {
	p.lastEvent = evt
	return nil
}
func (p *fakeRecordingEventProvider) Name() string       { return "Recording" }
func (p *fakeRecordingEventProvider) UsesEventDelivery() {}

// A late NotifyIncident after shutdown must be a no-op, not a send-on-closed
// panic: shutdown closes the provider channels and fanOut must never send on
// a closed channel (Fix 2e).
func TestNotifyIncidentAfterShutdownIsNoop(t *testing.T) {
	am := &AlertManager{}
	am.entries = []providerEntry{{
		provider: &fakeProvider{},
		retry:    retryConfig{maxAttempts: 1},
		ch:       make(chan deliverJob, channelCap),
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	am.Start(ctx)
	am.shutdown() // deterministic: closes provider channels, waits

	am.NotifyIncident(&model.Incident{
		Subject: model.Subject{
			Key:    "k",
			Name:   "n",
			Reason: "OOMKilled",
		},
	}, model.ActionCreate, nil)
}

func TestAlertManagerCanRestartAfterShutdown(t *testing.T) {
	am := &AlertManager{}
	am.entries = []providerEntry{{
		provider: &fakeProvider{},
		retry:    retryConfig{maxAttempts: 1},
		ch:       make(chan deliverJob, channelCap),
	}}

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

// Saturation: when a provider channel is full, fanOut drops the oldest queued
// job to make room and records it in the dead-letter queue instead of silently
// losing it.
