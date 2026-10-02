package signl4

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestSignl4RequiresTeamSecret(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	assert.Nil(t, NewSignl4(map[string]interface{}{}, "dev", deps))

	c := NewSignl4(map[string]interface{}{"teamSecret": "s"}, "dev", deps)
	assert.Equal(t, "SIGNL4", c.Name())
	assert.Equal(t, "https://connect.signl4.com/webhook/s", c.url)
}

func TestSignl4CustomURL(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	c := NewSignl4(map[string]interface{}{
		"url": "https://connect.example.com/webhook/", "teamSecret": "s",
	}, "dev", deps)
	assert.Equal(t, "https://connect.example.com/webhook/s", c.url)
}
