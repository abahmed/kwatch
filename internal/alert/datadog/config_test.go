package datadog

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestDatadogConfig(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	assert.Nil(t, NewDatadog(map[string]interface{}{}, "dev", deps))

	c := NewDatadog(map[string]interface{}{"apiKey": "k"}, "dev", deps)
	assert.Equal(t, "Datadog", c.Name())
	assert.Equal(t, "https://api.datadoghq.com/api/v1/events", c.url)

	c = NewDatadog(map[string]interface{}{
		"apiKey": "k", "site": "datadoghq.eu",
		"tags": []interface{}{"team:sre", "", 3},
	}, "dev", deps)
	assert.Equal(t, "https://api.datadoghq.eu/api/v1/events", c.url)
	assert.Equal(t, []string{"team:sre"}, c.tags)
}
