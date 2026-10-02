package sensugo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSensugoConfig(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	c := NewSensugo(map[string]interface{}{
		"url": "http://sensu.example.com:8080/", "apiKey": "k",
	}, "dev", deps)
	assert.Equal(t, "Sensu Go", c.Name())
	assert.Equal(t, "http://sensu.example.com:8080"+
		"/api/core/v2/namespaces/default/events", c.url)
	assert.Equal(t, "kwatch", c.entity)

	c = NewSensugo(map[string]interface{}{
		"url": "http://sensu", "apiKey": "k", "namespace": "ops",
		"entity": "agent",
	}, "dev", deps)
	assert.Equal(t, "http://sensu/api/core/v2/namespaces/ops/events", c.url)
	assert.Equal(t, "agent", c.entity)

	assert.Nil(t, NewSensugo(map[string]interface{}{"apiKey": "k"}, "dev",
		deps))
	assert.Nil(t, NewSensugo(
		map[string]interface{}{"url": "https://a.test"}, "dev", deps))
}
