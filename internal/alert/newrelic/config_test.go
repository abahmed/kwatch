package newrelic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestNewRelicConfig(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	c := NewNewRelic(map[string]interface{}{
		"apiKey": "k", "accountId": "123456",
	}, "dev", deps)
	assert.Equal(t, "New Relic", c.Name())
	assert.Equal(t, "https://insights-collector.newrelic.com"+
		"/v1/accounts/123456/events", c.url)

	assert.Nil(t, NewNewRelic(map[string]interface{}{}, "dev", deps))
	assert.Nil(t, NewNewRelic(map[string]interface{}{"apiKey": "k"},
		"dev", deps))
	assert.Nil(t, NewNewRelic(map[string]interface{}{"accountId": "1"},
		"dev", deps))
}
