package alertmanager

import (
	"errors"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
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
	alertmanager := AlertManager{}
	alertmanager.Init(nil, nil)
	assert.Len(alertmanager.providers, 0)
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

	alertmanager := AlertManager{}
	alertmanager.Init(alertMap, &config.App{ClusterName: "dev"})

	assert.Len(
		alertmanager.providers,
		len(alertMap),
		"get providers returned %d expected %d")
}

func TestSendProvidersEvent(t *testing.T) {
	alertmanager := AlertManager{}
	alertmanager.providers = append(
		alertmanager.providers,
		&fakeProvider{},
		&fakeProviderWithError{},
	)
	alertmanager.NotifyEvent(event.Event{})
}

func TestSendProvidersMsg(t *testing.T) {
	alertmanager := AlertManager{}
	alertmanager.providers = append(
		alertmanager.providers,
		&fakeProvider{},
		&fakeProviderWithError{},
	)
	alertmanager.Notify("hello world!")
}
