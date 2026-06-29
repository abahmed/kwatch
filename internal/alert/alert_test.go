package alert

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/llm"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
)

type fakeProvider struct{}

func (p *fakeProvider) SendMessage(msg string) error {
	return nil
}
func (p *fakeProvider) SendEvent(evt *event.Event) error {
	return nil
}
func (p *fakeProvider) Name() string {
	return "Slack"
}

type fakeProviderWithError struct{}

func (p *fakeProviderWithError) SendMessage(msg string) error {
	return errors.New("error")
}
func (p *fakeProviderWithError) SendEvent(evt *event.Event) error {
	return errors.New("error")
}
func (p *fakeProviderWithError) Name() string {
	return "Slack Error"
}

func TestAlertManagerNoConfig(t *testing.T) {
	assert := assert.New(t)
	am := AlertManager{}
	am.Init(nil, nil)
	assert.Len(am.entries, 0)
}

func TestGetProvidersUnknownSkipped(t *testing.T) {
	assert := assert.New(t)

	alertMap := map[string]map[string]interface{}{
		"slack":        {"webhook": "test"},
		"notaprovider": {"key": "val"},
	}

	am := AlertManager{}
	am.Init(alertMap, &config.App{ClusterName: "dev"})

	assert.Len(am.entries, 1)
}

func TestGetProviders(t *testing.T) {
	assert := assert.New(t)

	alertMap := map[string]map[string]interface{}{
		"slack": {
			"webhook": "test",
		},
		"pagerduty": {
			"integrationKey": "test",
		},
		"discord": {
			"webhook": "test/id",
		},
		"telegram": {
			"token":  "test",
			"chatId": "test",
		},
		"teams": {
			"webhook": "test",
		},
		"mattermost": {
			"webhook": "test",
		},
		"rocketchat": {
			"webhook": "test",
		},
		"opsgenie": {
			"apiKey": "test",
		},
		"email": {
			"from":     "test@test.com",
			"to":       "test2@test.com",
			"host":     "chat.google.com",
			"port":     "5432",
			"password": "test",
		},
		"matrix": {
			"homeServer":     "localhost",
			"accessToken":    "testToken",
			"internalRoomId": "room1",
		},
		"dingtalk": {
			"accessToken": "testToken",
		},
		"feishu": {
			"webhook": "test",
		},
		"webhook": {
			"url": "test",
		},
		"zenduty": {
			"integrationKey": "test",
		},
		"googlechat": {
			"webhook": "test",
		},
	}

	am := AlertManager{}
	am.Init(alertMap, &config.App{ClusterName: "dev"})

	assert.Len(
		am.entries,
		len(alertMap),
		"get providers returned %d expected %d")
}

func TestSendProvidersEvent(t *testing.T) {
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{provider: &fakeProvider{}, maxAttempts: 1},
		providerEntry{provider: &fakeProviderWithError{}, maxAttempts: 1},
	)
	am.NotifyEvent(event.Event{})
}

func TestSendProvidersMsg(t *testing.T) {
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{provider: &fakeProvider{}, maxAttempts: 1},
		providerEntry{provider: &fakeProviderWithError{}, maxAttempts: 1},
	)
	am.Notify("hello world!")
}

func TestNotifyIncidentCreate(t *testing.T) {
	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{provider: &fakeProvider{}, maxAttempts: 1})

	inc := &model.Incident{
		Key:       "default:deploy:CrashLoopBackOff",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
		Resource:  "pod",
		Count:     1,
		FirstSeen: time.Now().Add(-5 * time.Minute),
		LastSeen:  time.Now(),
		Resources: map[string]bool{"pod-1": true},
	}

	am.NotifyIncident(inc, model.ActionCreate)
}

func TestNotifyIncidentUpdate(t *testing.T) {
	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{provider: &fakeProvider{}, maxAttempts: 1}, providerEntry{provider: &fakeProviderWithError{}, maxAttempts: 1})

	inc := &model.Incident{
		Key:       "default:deploy:OOMKilled",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "OOMKilled",
		Resource:  "pod",
		Count:     3,
		FirstSeen: time.Now().Add(-10 * time.Minute),
		LastSeen:  time.Now(),
		Resources: map[string]bool{"pod-1": true, "pod-2": true},
	}

	am.NotifyIncident(inc, model.ActionUpdate)
}

func TestNotifyIncidentSkip(t *testing.T) {
	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{provider: &fakeProvider{}, maxAttempts: 1})

	inc := &model.Incident{
		Key:  "default:deploy:OOMKilled",
		Name: "deploy",
	}

	am.NotifyIncident(inc, model.ActionSkip)
}

// fakeThreadProvider implements both Provider and ThreadProvider
type fakeThreadProvider struct {
	lastInc *model.Incident
	lastAct model.IncidentAction
}

func (p *fakeThreadProvider) SendMessage(msg string) error     { return nil }
func (p *fakeThreadProvider) SendEvent(evt *event.Event) error { return nil }
func (p *fakeThreadProvider) Name() string                     { return "ThreadSlack" }
func (p *fakeThreadProvider) SendIncident(inc *model.Incident, action model.IncidentAction) error {
	p.lastInc = inc
	p.lastAct = action
	return nil
}

func TestNotifyIncidentCallsThreadProvider(t *testing.T) {
	tp := &fakeThreadProvider{}
	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{provider: tp, maxAttempts: 1})

	inc := &model.Incident{
		Key:  "default:deploy:OOMKilled",
		Name: "deploy",
	}
	am.NotifyIncident(inc, model.ActionCreate)

	assert.Equal(t, inc, tp.lastInc)
	assert.Equal(t, model.ActionCreate, tp.lastAct)
}

func TestNotifyIncidentThreadProviderWithSkip(t *testing.T) {
	tp := &fakeThreadProvider{}
	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{provider: tp, maxAttempts: 1})

	inc := &model.Incident{
		Key:  "default:deploy:OOMKilled",
		Name: "deploy",
	}
	am.NotifyIncident(inc, model.ActionSkip)

	assert.Nil(t, tp.lastInc)
}

func TestFormatIncidentMessage(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Key:           "default:deploy:CrashLoopBackOff",
		Name:          "deploy",
		Namespace:     "default",
		Reason:        "CrashLoopBackOff",
		Resource:      "pod",
		Count:         2,
		FirstSeen:     now.Add(-10 * time.Minute),
		LastSeen:      now,
		Resources:     map[string]bool{"pod-1": true, "pod-2": true},
		PeakResources: 2,
	}

	msg := formatIncidentMessage(inc, model.ActionCreate, 100, nil)
	assert.Contains(t, msg, "Incident")
	assert.Contains(t, msg, "deploy")
	assert.Contains(t, msg, "CrashLoopBackOff")
	assert.Contains(t, msg, "Peak: 2 resource")

	msgUpdate := formatIncidentMessage(inc, model.ActionUpdate, 100, nil)
	assert.Contains(t, msgUpdate, "Update")
}

func TestFormatIncidentMessageWithLogsEvents(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Key:           "default:deploy:CrashLoopBackOff",
		Name:          "deploy",
		Namespace:     "default",
		Reason:        "CrashLoopBackOff",
		Resource:      "pod",
		Count:         2,
		FirstSeen:     now.Add(-10 * time.Minute),
		LastSeen:      now,
		Resources:     map[string]bool{"pod-1": true, "pod-2": true},
		Logs:          "line1\nline2\nline3",
		Events:        "[2024-01-01] Pulling image\n[2024-01-01] BackOff restart",
		IncludeEvents: true,
		IncludeLogs:   true,
	}

	msg := formatIncidentMessage(inc, model.ActionCreate, 100, nil)
	assert.Contains(t, msg, "Logs:")
	assert.Contains(t, msg, "line1")
	assert.Contains(t, msg, "line2")
	assert.Contains(t, msg, "Events:")
	assert.Contains(t, msg, "Pulling image")
	assert.Contains(t, msg, "BackOff restart")
}

func TestFormatResolvedMessageGolden(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Key:       "default:deploy:OOMKilled",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "OOMKilled",
		Resource:  "pod",
		Count:     3,
		FirstSeen: now.Add(-20 * time.Minute),
		LastSeen:  now,
		Resources: map[string]bool{"pod-1": true},
	}

	msg := formatIncidentMessage(inc, model.ActionResolved, 100, nil)
	assert.Contains(t, msg, "Resolved")
	assert.Contains(t, msg, "deploy")
	assert.Contains(t, msg, "OOMKilled")
	assert.Contains(t, msg, "Total events: 3")
}

func TestSilenceByNamespace(t *testing.T) {
	am := AlertManager{}
	am.SetSilences([]config.SilenceRule{
		{Namespaces: []string{"kube-system"}},
	})

	inc := &model.Incident{
		Key:       "kube-system:pod:ImagePullBackOff",
		Name:      "pod",
		Namespace: "kube-system",
		Reason:    "ImagePullBackOff",
	}
	assert.True(t, am.isSilenced(inc))

	inc2 := &model.Incident{
		Key:       "default:pod:ImagePullBackOff",
		Name:      "pod",
		Namespace: "default",
		Reason:    "ImagePullBackOff",
	}
	assert.False(t, am.isSilenced(inc2))
}

func TestSilenceByReason(t *testing.T) {
	am := AlertManager{}
	am.SetSilences([]config.SilenceRule{
		{Reasons: []string{"BackOff"}},
	})

	inc := &model.Incident{
		Key:       "default:pod:BackOff",
		Name:      "pod",
		Namespace: "default",
		Reason:    "BackOff",
	}
	assert.True(t, am.isSilenced(inc))

	inc2 := &model.Incident{
		Key:       "default:pod:OOMKilled",
		Name:      "pod",
		Namespace: "default",
		Reason:    "OOMKilled",
	}
	assert.False(t, am.isSilenced(inc2))
}

func TestRouteFilter(t *testing.T) {
	routes := []config.AlertRoute{
		{Namespaces: []string{"production"}, Severities: []string{"high"}},
	}

	inc := &model.Incident{
		Key:       "production:pod:OOMKilled",
		Name:      "pod",
		Namespace: "production",
		Reason:    "OOMKilled",
		Severity:  "high",
	}
	assert.True(t, matchesRoute(routes[0], inc))

	inc2 := &model.Incident{
		Key:       "staging:pod:OOMKilled",
		Name:      "pod",
		Namespace: "staging",
		Reason:    "OOMKilled",
		Severity:  "high",
	}
	assert.False(t, matchesRoute(routes[0], inc2))

	inc3 := &model.Incident{
		Key:       "production:pod:BackOff",
		Name:      "pod",
		Namespace: "production",
		Reason:    "BackOff",
		Severity:  "normal",
	}
	assert.False(t, matchesRoute(routes[0], inc3))
}

func TestShouldDeliverNoRoutes(t *testing.T) {
	inc := &model.Incident{Key: "default:pod:Error"}
	assert.True(t, shouldDeliver(nil, inc))
	assert.True(t, shouldDeliver([]config.AlertRoute{}, inc))
}

func TestSetTemplates(t *testing.T) {
	am := AlertManager{}
	am.SetTemplates(map[string]string{
		"crashloopbackoff": "ALERT {{.Incident.Name}} — {{.Action}}",
	})
	if am.templates == nil {
		t.Fatal("templates map is nil")
	}
	if _, ok := am.templates["crashloopbackoff"]; !ok {
		t.Fatal("crashloopbackoff template not found")
	}
}

func TestSetTemplatesNil(t *testing.T) {
	am := AlertManager{}
	am.SetTemplates(nil)
	if am.templates != nil {
		t.Fatal("expected nil templates")
	}
	am.SetTemplates(map[string]string{})
	if am.templates != nil {
		t.Fatal("expected nil templates for empty map")
	}
}

func TestFormatIncidentMessageWithTemplate(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Key:       "default:deploy:CrashLoopBackOff",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
		Resource:  "pod",
		Count:     2,
		FirstSeen: now.Add(-10 * time.Minute),
		LastSeen:  now,
		Resources: map[string]bool{"pod-1": true},
	}

	am := AlertManager{}
	am.SetTemplates(map[string]string{
		"crashloopbackoff": "{{.Incident.Name}} {{.Action}}",
	})

	msg := formatIncidentMessage(inc, model.ActionCreate, 100, am.templates)
	want := "deploy create"
	if msg != want {
		t.Errorf("got %q, want %q", msg, want)
	}
}

func TestFormatIncidentMessageWithTemplateRenderError(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Key:       "default:deploy:OOMKilled",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "OOMKilled",
		Resource:  "pod",
		Count:     2,
		FirstSeen: now.Add(-10 * time.Minute),
		LastSeen:  now,
		Resources: map[string]bool{"pod-1": true},
	}

	am := AlertManager{}
	// bad template syntax — Parse will reject it, so it won't be stored
	am.SetTemplates(map[string]string{
		"oomkilled": "{{.Incident.Name {{.Action}}",
	})
	// no template stored -> falls back to default
	msg := formatIncidentMessage(inc, model.ActionCreate, 100, am.templates)
	if msg == "" {
		t.Fatal("expected fallback message, got empty")
	}
	if !strings.Contains(msg, "deploy") {
		t.Errorf("expected default message to contain pod name, got %q", msg)
	}
}

func TestFormatIncidentMessageUnregisteredReason(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Key:       "default:deploy:NodeNotReady",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "NodeNotReady",
		Resource:  "pod",
		Count:     1,
		FirstSeen: now.Add(-10 * time.Minute),
		LastSeen:  now,
		Resources: map[string]bool{"pod-1": true},
	}

	am := AlertManager{}
	am.SetTemplates(map[string]string{
		"crashloopbackoff": "OVERRIDE",
	})

	msg := formatIncidentMessage(inc, model.ActionCreate, 100, am.templates)
	if !strings.Contains(msg, "NodeNotReady") {
		t.Errorf("expected default message to contain reason, got %q", msg)
	}
}

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
		t.Errorf("expected pagerduty to have no fallback, got %v", pagerEntry.fallback)
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
func (p *errorRecorderProvider) SendEvent(evt *event.Event) error { return p.err }
func (p *errorRecorderProvider) Name() string                     { return p.name }

func TestFallbackUsedOnExhaustion(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary", err: nil}
	fb := &errorRecorderProvider{name: "Fallback", err: nil}

	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{
		provider:    primary,
		maxAttempts: 1,
		retryDelay:  time.Millisecond,
		fallback:    &providerEntry{provider: fb},
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
		t.Errorf("expected 1 primary call on failure, got %d", primary.callCount)
	}
	if fb.callCount != 1 {
		t.Errorf("expected 1 fallback call, got %d", fb.callCount)
	}
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
	attempts, delay, maxBackoff := extractRetry(cfg)
	assert.Equal(t, 3, attempts)
	assert.Equal(t, 2*time.Second, delay)
	assert.Equal(t, 10*time.Second, maxBackoff)
}

func TestExtractRetryJSONFloat(t *testing.T) {
	// JSON/CRD paths unmarshal numbers as float64.
	cfg := map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": float64(5),
		},
	}
	attempts, _, _ := extractRetry(cfg)
	assert.Equal(t, 5, attempts)
}

func TestExtractRetryClamps(t *testing.T) {
	cfg := map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": 0,
		},
	}
	attempts, _, _ := extractRetry(cfg)
	assert.Equal(t, 1, attempts)

	cfg = map[string]interface{}{
		"retry": map[string]interface{}{
			"maxAttempts": 100,
		},
	}
	attempts, _, _ = extractRetry(cfg)
	assert.Equal(t, 20, attempts)
}

func TestExtractRetryDefaults(t *testing.T) {
	attempts, delay, maxBackoff := extractRetry(map[string]interface{}{})
	assert.Equal(t, 1, attempts)
	assert.Equal(t, time.Second, delay)
	assert.Equal(t, defaultMaxBackoff, maxBackoff)
}

func TestSendWithRetryReturnsError(t *testing.T) {
	err := sendWithRetry(context.Background(), func() error {
		return errors.New("fail")
	}, 1, time.Millisecond, 0, "test")
	if err == nil {
		t.Fatal("expected error from sendWithRetry")
	}
}

func TestSendWithRetrySuccess(t *testing.T) {
	err := sendWithRetry(context.Background(), func() error {
		return nil
	}, 3, time.Millisecond, 0, "test")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestNotifyIncidentEventDeliveryProviderPropagatesActionAndDedup(t *testing.T) {
	fp := &fakeRecordingEventProvider{}
	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{provider: fp, maxAttempts: 1})

	inc := &model.Incident{
		Key:       "default:deploy:CrashLoopBackOff",
		Name:      "deploy",
		Namespace: "default",
		Reason:    "CrashLoopBackOff",
		Resource:  "pod",
		ID:        "abc123",
		Count:     1,
		FirstSeen: time.Now().Add(-5 * time.Minute),
		LastSeen:  time.Now(),
		Resources: map[string]bool{"pod-1": true},
	}

	am.NotifyIncident(inc, model.ActionResolved)

	if fp.lastEvent == nil {
		t.Fatal("expected SendEvent to be called")
	}
	assert.Equal(t, "resolved", fp.lastEvent.Action)
	assert.Equal(t, "abc123", fp.lastEvent.DedupKey)
}

func TestNotifyIncidentDigestFlushDelivered(t *testing.T) {
	fp := &fakeProvider{}
	tp := &fakeThreadProvider{}
	am := AlertManager{}
	am.entries = append(am.entries,
		providerEntry{provider: fp, maxAttempts: 1},
		providerEntry{provider: tp, maxAttempts: 1},
	)

	inc := &model.Incident{
		Key:    "digest:1234567890",
		Reason: "DigestSummary",
		Count:  5,
		Hint:   "⚡ 5 new incidents in 1m0s — OOMKilled × 3 (ns1, ns2); CrashLoopBackOff × 2 (ns1)",
	}

	am.NotifyIncident(inc, model.ActionDigestFlush)

	// ThreadProvider must NOT receive via SendIncident for digests
	assert.Nil(t, tp.lastInc, "ThreadProvider should not receive digest via SendIncident")
}

type fakeRecordingEventProvider struct {
	lastEvent *event.Event
}

func (p *fakeRecordingEventProvider) SendMessage(msg string) error { return nil }
func (p *fakeRecordingEventProvider) SendEvent(evt *event.Event) error {
	p.lastEvent = evt
	return nil
}
func (p *fakeRecordingEventProvider) Name() string       { return "Recording" }
func (p *fakeRecordingEventProvider) UsesEventDelivery() {}

// countingProvider signals each delivery so tests can synchronize without
// poking shutdown internals.
type countingProvider struct{ delivered chan struct{} }

func (p *countingProvider) Name() string                 { return "Slack" }
func (p *countingProvider) SendMessage(string) error     { p.delivered <- struct{}{}; return nil }
func (p *countingProvider) SendEvent(*event.Event) error { return nil }

// P2: in-flight enrichment must finish its fanOut on OPEN provider channels
// during shutdown — no send-on-closed panic, and the alert is still delivered.
func TestShutdownNoPanicWithInflightEnrichment(t *testing.T) {
	// Stub LLM sidecar: the handler blocks long enough for shutdown to start
	// closing channels, then returns (causing a JSON decode failure in
	// Analyze — the enrichment completes with error and fans out).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	delivered := make(chan struct{}, 8)
	am := &AlertManager{}
	am.entries = []providerEntry{{
		provider:    &countingProvider{delivered: delivered},
		maxAttempts: 1,
		ch:          make(chan deliverJob, channelCap),
	}}
	am.llm = llm.New(srv.URL)
	am.enrichCh = make(chan deliverJob, 1)

	ctx, cancel := context.WithCancel(context.Background())
	am.Start(ctx)

	am.NotifyIncident(&model.Incident{Key: "k", Name: "n", Reason: "OOMKilled"}, model.ActionCreate)
	time.Sleep(50 * time.Millisecond) // let the enrich worker enter Analyze
	cancel()                          // triggers shutdown (enrichCh closed)

	select {
	case <-delivered: // alert delivered even though enrichment was cut short
	case <-time.After(2 * time.Second):
		t.Fatal("incident was not delivered after shutdown")
	}
}

// P2 (second case): a late NotifyIncident after shutdown closed enrichCh must
// be a no-op, not a send-on-closed panic (Fix 2e).
func TestNotifyIncidentAfterShutdownIsNoop(t *testing.T) {
	am := &AlertManager{}
	am.entries = []providerEntry{{
		provider:    &fakeProvider{},
		maxAttempts: 1,
		ch:          make(chan deliverJob, channelCap),
	}}
	am.llm = llm.New("http://127.0.0.1:0")
	am.enrichCh = make(chan deliverJob, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	am.Start(ctx)
	am.shutdown() // deterministic: closes enrichCh + provider channels, waits

	// enrichCh is now closed; without Fix 2e this would panic.
	am.NotifyIncident(&model.Incident{Key: "k", Name: "n", Reason: "OOMKilled"}, model.ActionCreate)
}

// P3: the breaker is a true single-probe half-open (Fix 3). Tested directly —
// record/allow take an explicit `now`; enrichOne's time.Now() is not injectable.
func TestBreakerSingleProbe(t *testing.T) {
	var b breaker
	t0 := time.Now()

	b.record(t0, false)
	b.record(t0, false)
	b.record(t0, false) // threshold reached → open
	assert.False(t, b.allow(t0))
	assert.False(t, b.allow(t0.Add(breakerCooldown-time.Second)))

	probeAt := t0.Add(breakerCooldown + time.Second)
	assert.True(t, b.allow(probeAt)) // exactly one probe after cooldown

	b.record(probeAt, false) // failed probe re-opens for a full cooldown
	assert.False(t, b.allow(probeAt.Add(time.Second)))
	assert.False(t, b.allow(probeAt.Add(breakerCooldown-time.Second)))

	closeAt := probeAt.Add(breakerCooldown + time.Second)
	assert.True(t, b.allow(closeAt))
	b.record(closeAt, true) // successful probe closes
	assert.True(t, b.allow(closeAt))
	assert.Equal(t, 0, b.fails)
}
