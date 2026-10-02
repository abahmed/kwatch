package zenduty

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestZendutyRequiresIntegrationKey(t *testing.T) {
	rec := providertest.NewRecorder(t)
	assert.Nil(t, NewZenduty(map[string]interface{}{}, "dev",
		rec.Dependencies()))

	c := NewZenduty(map[string]interface{}{"integrationKey": "k"}, "dev",
		rec.Dependencies())
	assert.NotNil(t, c)
	assert.Equal(t, "Zenduty", c.Name())
}

func TestZendutyIgnoresInvalidAlertType(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewZenduty(map[string]interface{}{
		"integrationKey": "k", "alertType": "loud",
	}, "dev", rec.Dependencies())
	assert.Equal(t, "", c.alertType)
}
