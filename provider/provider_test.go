package provider

import (
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
)

func TestGetProviders(t *testing.T) {
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

	providers := GetProviders(&config.Config{Alert: alertMap})
	if len(providers) != len(alertMap) {
		t.Fatalf(
			"get providers returned %d expected %d",
			len(providers),
			len(alertMap))
	}
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
	providers := GetProviders(&config.Config{Alert: alertMap})

	SendProvidersEvent(providers, event.Event{})
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
	providers := GetProviders(&config.Config{Alert: alertMap})

	SendProvidersMsg(providers, "hello world!")
}
