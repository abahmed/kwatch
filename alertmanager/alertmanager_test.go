package alertmanager

import (
	"testing"

	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func TestAlertManagerNoConfig(t *testing.T) {
	assert := assert.New(t)
	alertmanager := AlertManager{}
	alertmanager.Init(nil)
	assert.Len(alertmanager.providers, 0)
}

func TestGetProviders(t *testing.T) {
	assert := assert.New(t)

	alertMap := map[string]map[string]string{
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
		"opsgenie": {
			"apiKey": "test",
		},
	}

	alertmanager := AlertManager{}
	alertmanager.Init(alertMap)

	assert.Len(
		alertmanager.providers,
		len(alertMap),
		"get providers returned %d expected %d")
}

func TestSendProvidersEvent(t *testing.T) {
	alertMap := map[string]map[string]string{
		"slack": {
			"webhook": "test",
		},
		"pagerduty": {
			"integrationkey": "test",
		},
		"discord": {
			"webhook": "test/test",
		},
		"telegram": {
			"token":  "test",
			"chatid": "test",
		},
		"teams": {
			"webhook": "test",
		},
		"rocketchat": {
			"webhook": "test",
		},
		"mattermost": {
			"webhook": "test",
		},
		"opsgenie": {
			"apiKey": "test",
		},
	}
	alertmanager := AlertManager{}
	alertmanager.Init(alertMap)
	alertmanager.NotifyEvent(event.Event{})
}

func TestSendProvidersMsg(t *testing.T) {
	alertMap := map[string]map[string]string{
		"slack": {
			"webhook": "test",
		},
		"pagerduty": {
			"integrationkey": "test",
		},
		"discord": {
			"webhook": "test",
		},
		"telegram": {
			"token":  "test",
			"chatid": "test",
		},
		"teams": {
			"webhook": "test",
		},
		"rocketchat": {
			"webhook": "test",
		},
		"mattermost": {
			"webhook": "test",
		},
		"opsgenie": {
			"apiKey": "test",
		},
	}

	alertmanager := AlertManager{}
	alertmanager.Init(alertMap)
	alertmanager.Notify("hello world!")
}
