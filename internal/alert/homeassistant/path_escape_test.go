package homeassistant

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The service is one path segment: a "/" or "?" in it must not change
// the route the token is sent to.
func TestHomeAssistantEscapesTheServiceSegment(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	c := NewHomeAssistant(map[string]interface{}{
		"token": "t", "url": "http://ha.corp:8123",
		"service": "mobile app/x?y",
	}, "dev", deps)
	require.NotNil(t, c)
	assert.Equal(t,
		"http://ha.corp:8123/api/services/notify/mobile%20app%2Fx%3Fy",
		c.url)
}
