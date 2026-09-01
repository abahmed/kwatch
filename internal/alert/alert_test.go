package alert

import (
	"errors"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

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
