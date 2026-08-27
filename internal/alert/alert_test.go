package alert

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// testBuildMessage is a helper for tests that creates a minimal AlertManager
// and calls buildMessage with no insight, 100 max lines, and no templates.
func testBuildMessage(
	inc *model.Incident,
	action model.IncidentAction,
	clusterName string,
) string {
	am := AlertManager{clusterName: clusterName}
	return am.buildMessage(inc, action, nil, nil)
}

// testBuildMessageWithTemplate is a helper for tests that builds a message
// with the given parsed templates.
func testBuildMessageWithTemplate(
	inc *model.Incident,
	action model.IncidentAction,
	clusterName string,
	rawTpls map[string]string,
) string {
	am := AlertManager{clusterName: clusterName}
	parsed := map[string]*template.Template{}
	for k, v := range rawTpls {
		t, err := template.New(k).Parse(v)
		if err == nil {
			parsed[k] = t
		}
	}
	return am.buildMessage(inc, action, nil, parsed)
}

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
		providerEntry{
			provider: &fakeProvider{},
			retry:    retryConfig{maxAttempts: 1},
		},
		providerEntry{
			provider: &fakeProviderWithError{},
			retry:    retryConfig{maxAttempts: 1},
		},
	)
	am.NotifyEvent(event.Event{})
}

func TestSendProvidersMsg(t *testing.T) {
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{
			provider: &fakeProvider{},
			retry:    retryConfig{maxAttempts: 1},
		},
		providerEntry{
			provider: &fakeProviderWithError{},
			retry:    retryConfig{maxAttempts: 1},
		},
	)
	am.Notify("hello world!")
}

func TestNotifyIncidentCreate(t *testing.T) {
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{
			provider: &fakeProvider{},
			retry:    retryConfig{maxAttempts: 1},
		},
	)

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     1,
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
			Resources: map[string]bool{"pod-1": true},
		},
	}

	am.NotifyIncident(inc, model.ActionCreate, nil)
}

func TestNotifyIncidentUpdate(t *testing.T) {
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{
			provider: &fakeProvider{},
			retry:    retryConfig{maxAttempts: 1},
		},
		providerEntry{
			provider: &fakeProviderWithError{},
			retry:    retryConfig{maxAttempts: 1},
		},
	)

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:OOMKilled",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "OOMKilled",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     3,
			FirstSeen: time.Now().Add(-10 * time.Minute),
			LastSeen:  time.Now(),
			Resources: map[string]bool{"pod-1": true, "pod-2": true},
		},
	}

	am.NotifyIncident(inc, model.ActionUpdate, nil)
}

func TestNotifyIncidentSkip(t *testing.T) {
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{
			provider: &fakeProvider{},
			retry:    retryConfig{maxAttempts: 1},
		},
	)

	inc := &model.Incident{
		Subject: model.Subject{
			Key:  "default:deploy:OOMKilled",
			Name: "deploy",
		},
	}

	am.NotifyIncident(inc, model.ActionSkip, nil)
}

// fakeThreadProvider implements both Provider and ThreadProvider
type fakeThreadProvider struct {
	lastInc *model.Incident
	lastAct model.IncidentAction
}

func (p *fakeThreadProvider) SendMessage(msg string) error     { return nil }
func (p *fakeThreadProvider) SendEvent(evt *event.Event) error { return nil }

func (p *fakeThreadProvider) Name() string { return "ThreadSlack" }

func (p *fakeThreadProvider) SendIncident(
	inc *model.Incident,
	action model.IncidentAction,
) error {
	p.lastInc = inc
	p.lastAct = action
	return nil
}

func TestNotifyIncidentCallsThreadProvider(t *testing.T) {
	tp := &fakeThreadProvider{}
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{provider: tp, retry: retryConfig{maxAttempts: 1}},
	)

	inc := &model.Incident{
		Subject: model.Subject{
			Key:  "default:deploy:OOMKilled",
			Name: "deploy",
		},
	}

	am.NotifyIncident(inc, model.ActionCreate, nil)

	assert.Equal(t, inc, tp.lastInc)
	assert.Equal(t, model.ActionCreate, tp.lastAct)
}

func TestNotifyIncidentThreadProviderWithSkip(t *testing.T) {
	tp := &fakeThreadProvider{}
	am := AlertManager{}
	am.entries = append(
		am.entries,
		providerEntry{provider: tp, retry: retryConfig{maxAttempts: 1}},
	)

	inc := &model.Incident{
		Subject: model.Subject{
			Key:  "default:deploy:OOMKilled",
			Name: "deploy",
		},
	}

	am.NotifyIncident(inc, model.ActionSkip, nil)

	assert.Nil(t, tp.lastInc)
}

func TestNotifyIncidentThreadProviderClamped(t *testing.T) {
	tp := &fakeThreadProvider{}
	am := AlertManager{}
	am.entries = append(am.entries, providerEntry{
		provider: tp,
		retry: retryConfig{
			maxAttempts: 1,
			delay:       time.Second,
			maxBackoff:  defaultMaxBackoff,
		},
		maxBytes: 2000,
	})

	bigLog := strings.Repeat("error: something failed\n", 300)
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     2,
			FirstSeen: time.Now().Add(-5 * time.Minute),
			LastSeen:  time.Now(),
			Resources: map[string]bool{"pod-1": true},
		},
		Evidence: model.Evidence{
			Logs:        bigLog,
			IncludeLogs: true,
		},
	}

	am.NotifyIncident(inc, model.ActionCreate, nil)

	assert.NotNil(t, tp.lastInc)
	assert.Less(t, len(tp.lastInc.Logs), len(bigLog))
	assert.Contains(t, tp.lastInc.Logs, "truncated")

	rendered := testBuildMessage(tp.lastInc, model.ActionCreate, "")
	assert.LessOrEqual(t, len(rendered), 2000)
}

func TestDefaultMaxBytes(t *testing.T) {
	assert.Equal(t, 2000, defaultMaxBytes("discord"))
	assert.Equal(t, 4096, defaultMaxBytes("telegram"))
	assert.Equal(t, 28000, defaultMaxBytes("teams"))
	assert.Equal(t, 40000, defaultMaxBytes("slack"))
	assert.Equal(t, 0, defaultMaxBytes("webhook"))
}

func TestFormatIncidentMessage(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:         2,
			FirstSeen:     now.Add(-10 * time.Minute),
			LastSeen:      now,
			Resources:     map[string]bool{"pod-1": true, "pod-2": true},
			PeakResources: 2,
		},
	}

	msg := testBuildMessage(inc, model.ActionCreate, "test-cluster")
	assert.Contains(t, msg, "CrashLoopBackOff")
	assert.Contains(t, msg, "deploy")
	assert.Contains(t, msg, "2")

	msgUpdate := testBuildMessage(inc, model.ActionUpdate, "test-cluster")
	assert.Contains(t, msgUpdate, "CrashLoopBackOff")
	assert.Contains(t, msgUpdate, "2")
}

func TestFormatIncidentMessageWithLogsEvents(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     2,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true, "pod-2": true},
		},
		Evidence: model.Evidence{
			Logs: "line1\nline2\nline3",
			Events: "[2024-01-01] Pulling image\n[2024-01-01] BackOff " +
				"restart",
			IncludeEvents: true,
			IncludeLogs:   true,
		},
	}

	msg := testBuildMessage(inc, model.ActionCreate, "test-cluster")
	assert.Contains(t, msg, "Logs")
	assert.Contains(t, msg, "line1")
	assert.Contains(t, msg, "line2")
	assert.Contains(t, msg, "Events")
	assert.Contains(t, msg, "Pulling image")
	assert.Contains(t, msg, "BackOff restart")
}

func TestFormatResolvedMessageGolden(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:OOMKilled",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "OOMKilled",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     3,
			FirstSeen: now.Add(-20 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	msg := testBuildMessage(inc, model.ActionResolved, "test-cluster")
	assert.Contains(t, msg, "Resolved")
	assert.Contains(t, msg, "deploy")
	assert.Contains(t, msg, "OOMKilled")
}

func TestSilenceByNamespace(t *testing.T) {
	am := AlertManager{}
	am.SetSilences([]config.SilenceRule{
		{Namespaces: []string{"kube-system"}},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "kube-system:pod:ImagePullBackOff",
			Name:      "pod",
			Namespace: "kube-system",
			Reason:    "ImagePullBackOff",
		},
	}

	assert.True(t, am.isSilenced(inc))

	inc2 := &model.Incident{
		Subject: model.Subject{
			Key:       "default:pod:ImagePullBackOff",
			Name:      "pod",
			Namespace: "default",
			Reason:    "ImagePullBackOff",
		},
	}

	assert.False(t, am.isSilenced(inc2))
}

func TestSilenceByReason(t *testing.T) {
	am := AlertManager{}
	am.SetSilences([]config.SilenceRule{
		{Reasons: []string{"BackOff"}},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:pod:BackOff",
			Name:      "pod",
			Namespace: "default",
			Reason:    "BackOff",
		},
	}

	assert.True(t, am.isSilenced(inc))

	inc2 := &model.Incident{
		Subject: model.Subject{
			Key:       "default:pod:OOMKilled",
			Name:      "pod",
			Namespace: "default",
			Reason:    "OOMKilled",
		},
	}

	assert.False(t, am.isSilenced(inc2))
}

func TestRouteFilter(t *testing.T) {
	routes := []config.AlertRoute{
		{Namespaces: []string{"production"}, Severities: []string{"high"}},
	}

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "production:pod:OOMKilled",
			Name:      "pod",
			Namespace: "production",
			Reason:    "OOMKilled",
		},
		Status: model.Status{
			Severity: "high",
		},
	}

	assert.True(t, matchesRoute(routes[0], inc))

	inc2 := &model.Incident{
		Subject: model.Subject{
			Key:       "dev:pod:OOMKilled",
			Name:      "pod",
			Namespace: "dev",
			Reason:    "OOMKilled",
		},
		Status: model.Status{
			Severity: "high",
		},
	}

	assert.False(t, matchesRoute(routes[0], inc2))

	inc3 := &model.Incident{
		Subject: model.Subject{
			Key:       "production:pod:BackOff",
			Name:      "pod",
			Namespace: "production",
			Reason:    "BackOff",
		},
		Status: model.Status{
			Severity: "normal",
		},
	}

	assert.False(t, matchesRoute(routes[0], inc3))
}

func TestShouldDeliverNoRoutes(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{
			Key: "default:pod:Error",
		},
	}
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
		Subject: model.Subject{
			Key:       "default:deploy:CrashLoopBackOff",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "CrashLoopBackOff",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     2,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	msg := testBuildMessageWithTemplate(
		inc,
		model.ActionCreate,
		"test-cluster",
		map[string]string{
			"crashloopbackoff": "{{.Incident.Name}} {{.Action}}",
		},
	)
	want := "deploy create"
	if msg != want {
		t.Errorf("got %q, want %q", msg, want)
	}
}

func TestFormatIncidentMessageWithTemplateRenderError(t *testing.T) {
	now := time.Now()
	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "default:deploy:OOMKilled",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "OOMKilled",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     2,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	// bad template syntax — Parse will reject it, so it won't be stored
	msg := testBuildMessageWithTemplate(
		inc,
		model.ActionCreate,
		"test-cluster",
		map[string]string{
			"oomkilled": "{{.Incident.Name {{.Action}}",
		},
	)
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
		Subject: model.Subject{
			Key:       "default:deploy:NodeNotReady",
			Name:      "deploy",
			Namespace: "default",
			Reason:    "NodeNotReady",
			Resource:  "pod",
		},
		Status: model.Status{
			Count:     1,
			FirstSeen: now.Add(-10 * time.Minute),
			LastSeen:  now,
			Resources: map[string]bool{"pod-1": true},
		},
	}

	msg := testBuildMessageWithTemplate(
		inc,
		model.ActionCreate,
		"test-cluster",
		map[string]string{
			"crashloopbackoff": "OVERRIDE",
		},
	)
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

// Saturation: when a provider channel is full, fanOut drops the oldest queued
// job to make room and records it in the dead-letter queue instead of silently
// losing it.
func TestFanOutSaturatedQueueRecordsDeadLetter(t *testing.T) {
	am := &AlertManager{}
	ch := make(chan deliverJob, channelCap)
	inc := &model.Incident{
		Subject: model.Subject{
			Key:    "arriving-job",
			Name:   "n1",
			Reason: "Error",
		},
	}
	for i := 0; i < channelCap; i++ {
		ch <- deliverJob{inc: &model.Incident{
			Subject: model.Subject{
				Key: model.IncidentKey(fmt.Sprintf("queued-%d", i)),
			},
		}}
	}
	am.entries = []providerEntry{{
		provider: &fakeProvider{},
		ch:       ch,
	}}

	am.mu.Lock()
	am.fanOut(deliverJob{inc: inc, action: model.ActionCreate})
	am.mu.Unlock()

	// Under saturation the already-queued notifications are kept — they are
	// the earlier, more diagnostic ones — and the arriving job is dead-
	// lettered instead. Evicting the oldest could discard an incident's
	// CREATE while keeping a later UPDATE for the same incident.
	assert.Len(t, ch, channelCap, "queue stays full; nothing was evicted")

	first := <-ch
	assert.Equal(t, model.IncidentKey("queued-0"), first.inc.Key,
		"the earliest queued notification must survive")

	dl := am.DeadLetters()
	dlList, ok := dl.([]DeadLetterEntry)
	require.True(t, ok)
	assert.Len(t, dlList, 1)
	assert.Equal(t, "arriving-job", dlList[0].Key,
		"the job that could not be queued is the one recorded")
	assert.Contains(t, dlList[0].Error, "queue saturated")
}

// A permanent failure — a malformed payload, a revoked token — fails the same
// way on every attempt, and each retry holds up every alert queued behind it
// on that provider. It must be given up on immediately.
func TestSendWithRetryStopsOnPermanentError(t *testing.T) {
	attempts := 0
	err := sendWithRetry(context.Background(), func() error {
		attempts++
		return event.Permanent(errors.New("invalid_blocks"))
	}, retryConfig{maxAttempts: 5, delay: time.Millisecond}, "test")
	require.Error(t, err)
	assert.Equal(t, 1, attempts, "a permanent error must not be retried")
	assert.True(
		t,
		event.IsPermanent(err),
		"the permanent marker must survive to the caller",
	)

	// A transient error still gets every attempt.
	attempts = 0
	_ = sendWithRetry(context.Background(), func() error {
		attempts++
		return errors.New("connection reset")
	}, retryConfig{maxAttempts: 3, delay: time.Millisecond}, "test")
	assert.Equal(t, 3, attempts, "a transient error is retried to maxAttempts")
}

type fakeInsightProvider struct {
	fakeProvider
	gotInsight   *insight.Insight
	plainCalled  bool
	insightCalls int
}

func (p *fakeInsightProvider) SendIncident(
	*model.Incident,
	model.IncidentAction,
) error {
	p.plainCalled = true
	return nil
}

func (p *fakeInsightProvider) SendIncidentWithInsight(
	_ *model.Incident,
	_ model.IncidentAction,
	ins *insight.Insight,
) error {
	p.insightCalls++
	p.gotInsight = ins
	return nil
}

// A provider that can render a diagnosis must be handed one. Falling back to
// the plain SendIncident would silently drop the cause, impact and changes.
func TestDeliverOnePrefersInsightCapableProvider(t *testing.T) {
	fp := &fakeInsightProvider{}
	am := &AlertManager{}
	entry := providerEntry{provider: fp, retry: retryConfig{maxAttempts: 1}}
	inc := &model.Incident{
		Subject: model.Subject{
			Key:    "ns:dep:Error:",
			Reason: "Error",
			Name:   "dep",
		},
	}
	ins := &insight.Insight{
		Cause:   "node worker-2 may be unhealthy",
		Pattern: "node_failure",
	}

	am.deliverOne(context.Background(), &entry, inc, model.ActionCreate, ins)

	assert.Equal(t, 1, fp.insightCalls)
	assert.False(
		t,
		fp.plainCalled,
		"the insight-aware path must be used, not the plain one",
	)
	require.NotNil(t, fp.gotInsight)
	assert.Equal(t, "node worker-2 may be unhealthy", fp.gotInsight.Cause)
}
