package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/notification"
)

type fakeProvider struct{}

func (p *fakeProvider) SendMessage(context.Context, string) error {
	return nil
}
func (p *fakeProvider) SendIncident(
	context.Context, notification.Message,
) error {
	return nil
}
func (p *fakeProvider) Name() string {
	return "Slack"
}

type fakeProviderWithError struct{}

func (p *fakeProviderWithError) SendMessage(context.Context, string) error {
	return errors.New("error")
}
func (p *fakeProviderWithError) SendIncident(
	_ context.Context,
	_ notification.Message,
) error {
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
		"slack":        {"webhook": "https://hooks.example.test/x"},
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
			"webhook": "https://hooks.example.test/x",
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
			"webhook": "https://hooks.example.test/x",
		},
		"mattermost": {
			"webhook": "https://hooks.example.test/x",
		},
		"rocketchat": {
			"webhook": "https://hooks.example.test/x",
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
			"homeServer":     "https://matrix.example.test",
			"accessToken":    "testToken",
			"internalRoomId": "room1",
		},
		"dingtalk": {
			"accessToken": "testToken",
		},
		"feishu": {
			"webhook": "https://hooks.example.test/x",
		},
		"webhook": {
			"url": "https://receiver.example.test/x",
		},
		"zenduty": {
			"integrationKey": "test",
		},
		"googlechat": {
			"webhook": "https://hooks.example.test/x",
		},
	}

	am := *newTestManager()
	err := initTestManager(
		&am,
		alertMap, &config.App{ClusterName: "dev"}, catalog.NewProvider,
	)
	assert.NoError(err)

	assert.Len(
		managerEntries(&am),
		len(alertMap),
		"get providers returned %d expected %d")
}

func TestSendProvidersIncident(t *testing.T) {
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
	am.NotifyIncident(*incidentJob("k", "default").incident)
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

func TestInitRuntimeFailsForBrokenProviderSettings(t *testing.T) {
	for name, settings := range map[string]map[string]interface{}{
		"slack":   {"webhook": "not a url"},
		"webhook": {"url": "ftp://receiver.example.test"},
		"teams":   {},
	} {
		t.Run(name, func(t *testing.T) {
			am := newTestManager()
			err := initTestManager(am,
				map[string]map[string]interface{}{name: settings},
				&config.App{ClusterName: "dev"}, catalog.NewProvider)

			require.Error(t, err)
			assert.Contains(t, err.Error(),
				"could not be constructed; check its settings")
			assert.Empty(t, managerEntries(am))
		})
	}
}
