package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func testRetryConfig(settings map[string]interface{}) retryConfig {
	runtime := config.CompileRuntimeConfig(&config.Config{
		Alert: map[string]map[string]interface{}{"test": settings},
	})
	providers := runtime.Delivery().Providers()
	return retryConfigFromRuntime(providers[0].Retry)
}

func TestFallbackResolve(t *testing.T) {
	am := *newTestManager()
	initTestManager(&am, map[string]map[string]interface{}{
		"slack": {
			"webhook":  "test",
			"fallback": "pagerduty",
		},
		"pagerduty": {
			"integrationKey": "test",
		},
	}, &config.App{ClusterName: "dev"}, catalog.NewProvider)

	entries := managerEntries(&am)
	var slackEntry, pagerEntry *providerEntry
	for i := range entries {
		switch entries[i].provider.Name() {
		case "Slack":
			slackEntry = &entries[i]
		case "PagerDuty":
			pagerEntry = &entries[i]
		}
	}
	if slackEntry == nil {
		t.Fatal("Slack entry not found")
	}
	if pagerEntry == nil {
		t.Fatal("PagerDuty entry not found")
	}
	if slackEntry.fallbackName != "pagerduty" {
		t.Errorf("expected slack fallback name pagerduty, got %q",
			slackEntry.fallbackName)
	}
	if pagerEntry.fallbackName != "" {
		t.Errorf("expected pagerduty to have no fallback")
	}
}

func TestFallbackResolveUnknown(t *testing.T) {
	am := *newTestManager()
	initTestManager(&am, map[string]map[string]interface{}{
		"slack": {
			"webhook":  "test",
			"fallback": "nonexistent",
		},
	}, &config.App{ClusterName: "dev"}, catalog.NewProvider)

	entries := managerEntries(&am)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].fallbackName != "nonexistent" {
		t.Errorf("expected unresolved fallback name to be retained")
	}
}

// errorRecorderProvider records calls and optionally returns errors
type errorRecorderProvider struct {
	name      string
	msg       string
	err       error
	callCount int
}

func (p *errorRecorderProvider) SendMessage(
	_ context.Context,
	msg string,
) error {
	p.msg = msg
	p.callCount++
	return p.err
}

func (p *errorRecorderProvider) SendEvent(
	_ context.Context,
	evt *event.Event,
) error {
	return p.err
}

func (p *errorRecorderProvider) Name() string { return p.name }

func TestFallbackUsedOnExhaustion(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary", err: nil}
	fb := &errorRecorderProvider{name: "Fallback", err: nil}

	am := *newTestManager()
	setManagerEntries(&am, []providerEntry{
		{
			provider:     primary,
			retry:        retryConfig{maxAttempts: 1, delay: time.Millisecond},
			fallbackName: "Fallback",
		},
		{provider: fb},
	})

	// primary succeeds — fallback should NOT be called
	entries := managerEntries(&am)
	am.deliverOne(context.Background(), &entries[0], deliverJob{
		kind: jobMessage,
		msg:  "test message",
	})
	if primary.callCount != 1 {
		t.Errorf("expected 1 primary call, got %d", primary.callCount)
	}
	// Now make primary fail
	primary.err = errors.New("fail")
	primary.callCount = 0
	entries = managerEntries(&am)
	am.deliverOne(context.Background(), &entries[0], deliverJob{
		kind: jobMessage,
		msg:  "test message 2",
	})
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

	am := *newTestManager()
	setManagerEntries(&am, []providerEntry{
		{
			provider:     primary,
			retry:        retryConfig{maxAttempts: 1, delay: time.Millisecond},
			fallbackName: "Fallback",
		},
		{provider: fb, maxBytes: 64},
	})

	primary.err = errors.New("fail")
	entries := managerEntries(&am)
	am.deliverOne(context.Background(), &entries[0], deliverJob{
		kind: jobMessage,
		msg:  strings.Repeat("x", 500),
	})

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

func (p *eventFallbackProvider) SendEvent(
	_ context.Context,
	_ *event.Event,
) error {
	p.eventCalls++
	return nil
}

func (p *eventFallbackProvider) SendMessage(context.Context, string) error {
	p.messageCalls++
	return nil
}

func (p *eventFallbackProvider) UsesEventDelivery() {}

func TestIncidentFallbackUsesEventDeliveryInterface(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary", err: errors.New("fail")}
	fallback := &eventFallbackProvider{}
	am := *managerWithEntries([]providerEntry{
		{
			provider:     primary,
			retry:        retryConfig{maxAttempts: 1, delay: time.Millisecond},
			fallbackName: "Event Fallback",
		},
		{
			provider: fallback,
			retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
		},
	})

	am.deliverOne(
		context.Background(), &managerEntries(&am)[0], incidentJob(&model.Incident{
			Subject: model.Subject{Key: "ns:pod:Error", Reason: "Error"},
		}, model.ActionCreate, nil),
	)

	assert.Equal(t, 1, fallback.eventCalls)
	assert.Zero(t, fallback.messageCalls)
}

func TestIncidentFallbackHonorsFallbackRoutes(t *testing.T) {
	primary := &errorRecorderProvider{
		name: "Primary",
		err:  errors.New("fail"),
	}
	fallback := &errorRecorderProvider{name: "Fallback"}
	am := *managerWithEntries([]providerEntry{
		{
			provider:     primary,
			retry:        retryConfig{maxAttempts: 1, delay: time.Millisecond},
			fallbackName: "Fallback",
		},
		{
			provider: fallback,
			routes: []config.AlertRoute{{
				Namespaces: []string{"ops"},
			}},
			retry: retryConfig{
				maxAttempts: 1,
				delay:       time.Millisecond,
			},
		},
	})

	am.deliverOne(
		context.Background(), &managerEntries(&am)[0], incidentJob(&model.Incident{
			Subject: model.Subject{
				Key:       "default:pod:Error",
				Namespace: "default",
				Reason:    "Error",
			},
		}, model.ActionCreate, nil),
	)

	assert.Zero(t, fallback.callCount)
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
	rc := testRetryConfig(cfg)
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
	rc := testRetryConfig(cfg)
	assert.Equal(t, 5, rc.maxAttempts)
}

func TestExtractRetryClamps(t *testing.T) {
	cfg := map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": 0,
		},
	}
	rc := testRetryConfig(cfg)
	assert.Equal(t, 1, rc.maxAttempts)

	cfg = map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": 100,
		},
	}
	rc = testRetryConfig(cfg)
	assert.Equal(t, 20, rc.maxAttempts)
}

func TestExtractRetryDefaults(t *testing.T) {
	rc := testRetryConfig(map[string]interface{}{})
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

func TestSendWithRetryNormalizesEmptyConfig(t *testing.T) {
	attempts := 0
	err := sendWithRetry(context.Background(), func() error {
		attempts++
		return errors.New("failed")
	}, retryConfig{}, "test")

	require.Error(t, err)
	assert.Equal(t, 1, attempts)
}

func TestFlushDigestUsesEventDelivery(t *testing.T) {
	provider := &eventFallbackProvider{}
	am := *newTestManager()
	entry := &providerEntry{
		provider: provider,
		retry: retryConfig{
			maxAttempts: 1,
			delay:       time.Millisecond,
		},
	}
	am.digestAdd(provider.Name(), incidentJob(&model.Incident{
		Subject: model.Subject{Reason: "Error"},
	}, model.ActionCreate, nil))

	am.flushDigest(context.Background(), entry)

	assert.Equal(t, 1, provider.eventCalls)
	assert.Zero(t, provider.messageCalls)
}

func TestFlushDigestRestoresAfterFailure(t *testing.T) {
	provider := &errorRecorderProvider{name: "Digest", err: errors.New("failed")}
	am := *newTestManager()
	entry := &providerEntry{
		provider: provider,
		retry: retryConfig{
			maxAttempts: 1,
			delay:       time.Millisecond,
		},
	}
	am.digestAdd(provider.Name(), incidentJob(&model.Incident{
		Subject: model.Subject{Reason: "Error"},
	}, model.ActionCreate, nil))

	am.flushDigest(context.Background(), entry)

	am.pacer.mu.Lock()
	state := am.pacer.digests[provider.Name()]
	am.pacer.mu.Unlock()
	require.NotNil(t, state)
	assert.Equal(t, 1, state.total)
}
