package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

type fakeProvider struct{}

func (p *fakeProvider) SendMessage(context.Context, string) error {
	return nil
}
func (p *fakeProvider) SendEvent(context.Context, *event.Event) error {
	return nil
}
func (p *fakeProvider) Name() string {
	return "Slack"
}

type fakeProviderWithError struct{}

func (p *fakeProviderWithError) SendMessage(context.Context, string) error {
	return errors.New("error")
}
func (p *fakeProviderWithError) SendEvent(context.Context, *event.Event) error {
	return errors.New("error")
}
func (p *fakeProviderWithError) Name() string {
	return "Slack Error"
}

func TestManagerNoConfig(t *testing.T) {
	assert := assert.New(t)
	am := *newTestManager()
	am.InitRuntime(config.RuntimeConfigFor(&config.Config{}), nil)
	assert.Len(managerEntries(&am), 0)
}

func TestGetProvidersRejectsUnknown(t *testing.T) {
	assert := assert.New(t)

	alertMap := map[string]map[string]interface{}{
		"slack":        {"webhook": "test"},
		"notaprovider": {"key": "val"},
	}

	am := *newTestManager()
	err := initTestManager(
		&am,
		alertMap, &config.App{ClusterName: "dev"}, catalog.NewProvider,
	)

	assert.Error(err)
	assert.Empty(managerEntries(&am))
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

	am := *newTestManager()
	initTestManager(
		&am,
		alertMap, &config.App{ClusterName: "dev"}, catalog.NewProvider,
	)

	assert.Len(
		managerEntries(&am),
		len(alertMap),
		"get providers returned %d expected %d")
}

func TestSendProvidersEvent(t *testing.T) {
	am := *newTestManager()
	appendManagerEntries(&am, providerEntry{
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
	am := *newTestManager()
	appendManagerEntries(&am, providerEntry{
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
