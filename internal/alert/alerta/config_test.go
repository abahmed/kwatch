package alerta

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestAlertaConfigDefaults(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	c := NewAlerta(map[string]interface{}{
		"url": "https://alerta.example.com/", "apiKey": "k",
	}, "dev", deps)
	assert.Equal(t, "Alerta", c.Name())
	assert.Equal(t, "https://alerta.example.com/api/alert", c.url)
	assert.Equal(t, "Production", c.environment)
	assert.Equal(t, "kwatch", c.service)
}

func TestAlertaRequiresURLAndKey(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	assert.Nil(t, NewAlerta(map[string]interface{}{}, "dev", deps))
	assert.Nil(t, NewAlerta(map[string]interface{}{"apiKey": "k"}, "dev",
		deps))
	assert.Nil(t, NewAlerta(map[string]interface{}{"url": "https://a.test"},
		"dev", deps))
}
