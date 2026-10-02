package splunkoncall

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

var testDeps = transport.Dependencies{
	HTTPClient: http.DefaultClient,
}

func testAppConfig() string {
	return "dev"
}

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSplunkOncall(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestSplunkOncall(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Splunk OnCall")
	assert.Equal(c.url, "https://alert.victorops.com/integrations/generic/20131114/alert/everyone/test")
}

func TestSplunkOncallCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":        "https://alert.example.com/integrations/generic/20131114/alert",
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "https://alert.example.com/integrations/generic/20131114/alert/everyone/test")
}

func TestSplunkOncallInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSplunkOncall(
		map[string]interface{}{
			"routingKey": "r",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewSplunkOncall(
		map[string]interface{}{
			"apiKey": "a",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}
