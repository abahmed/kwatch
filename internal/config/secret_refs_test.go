package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigResolvesProviderSecretFile(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "slack-webhook")
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(
		secretPath,
		[]byte("https://hooks.example.test/a:$value\n"),
		0o600,
	))
	raw := fmt.Sprintf(`alert:
  slack:
    webhook: "${file:%s}"
`, secretPath)
	require.NoError(t, os.WriteFile(configPath, []byte(raw), 0o600))
	t.Setenv("CONFIG_FILE", configPath)

	cfg, err := LoadConfig()

	require.NoError(t, err)
	assert.Equal(
		t,
		"https://hooks.example.test/a:$value",
		cfg.Alert["slack"]["webhook"],
	)
}

func TestLoadConfigRejectsPlainProviderSecret(t *testing.T) {
	cfg, err := testConfigFile(t, `alert:
  slack:
    webhook: "https://hooks.example.test/plain"
`)

	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "alert.slack.webhook is sensitive")
}

func TestLoadConfigRejectsEnvironmentProviderSecret(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK", "https://hooks.example.test/from-env")
	cfg, err := testConfigFile(t, `alert:
  slack:
    webhook: "${SLACK_WEBHOOK}"
`)

	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "must use an absolute ${file:/path}")
}

func TestLoadConfigRejectsPlainGlobalSecrets(t *testing.T) {
	cfg, err := testConfigFile(t, `healthCheck:
  diagnosticsToken: plain-token
heartbeatMonitor:
  url: https://heartbeat.example.test/secret-path
`)

	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "healthCheck.diagnosticsToken")
	assert.ErrorContains(t, err, "heartbeatMonitor.url")
}

func TestLoadConfigRequiresSecretWebhookHeaderValues(t *testing.T) {
	cfg, err := testConfigFile(t, `alert:
  webhook:
    url: "${file:/config/webhook-url}"
    headers:
      - name: Authorization
        value: plain-token
`)

	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "alert.webhook.headers[0].value")
}

func TestLoadConfigRejectsRelativeFileReference(t *testing.T) {
	cfg, err := testConfigFile(t, `alert:
  telegram:
    token: "${file:telegram-token}"
    chatId: "123"
`)

	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "must use an absolute ${file:/path}")
}

func TestLoadConfigRejectsYAMLAliasBypass(t *testing.T) {
	cfg, err := testConfigFile(t, `provider: &provider
  webhook: https://hooks.example.test/plain
alert:
  slack:
    <<: *provider
`)

	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "aliases and merge keys are not allowed")
}

func TestProviderCatalogSecretFlagsMatchRuntimePolicy(t *testing.T) {
	for _, field := range ProviderCatalog() {
		raw := providerCatalogTestConfig(field, "plain-value")
		err := validateSecretReferences(raw)
		if field.Secret {
			assert.Error(t, err, field.Provider+"."+field.Field)
			continue
		}
		assert.NoError(t, err, field.Provider+"."+field.Field)
	}
}

func TestProviderCatalogCoversKnownProviders(t *testing.T) {
	covered := make(map[string]bool)
	for _, field := range ProviderCatalog() {
		covered[field.Provider] = true
	}
	for provider := range KnownProviders {
		if provider == "incident.io" {
			continue
		}
		assert.True(t, covered[provider], "missing provider %s", provider)
	}
}

func providerCatalogTestConfig(field ProviderField, value string) string {
	parts := strings.Split(field.Field, ".")
	var body strings.Builder
	for i, part := range parts {
		body.WriteString(strings.Repeat(" ", 4+i*2))
		body.WriteString(part)
		if i == len(parts)-1 {
			body.WriteString(": ")
			body.WriteString(value)
			body.WriteByte('\n')
			continue
		}
		body.WriteString(":\n")
	}
	return "alert:\n  " + field.Provider + ":\n" + body.String()
}
