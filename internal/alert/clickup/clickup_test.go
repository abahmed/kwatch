package clickup

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

	c := NewClickup(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestClickup(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":  "test",
		"listId": "abc123",
	}
	c := NewClickup(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Clickup")
	assert.Equal(c.url, "https://api.clickup.com/api/v2/list/abc123/task")
}

func TestClickupInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewClickup(
		map[string]interface{}{
			"listId": "abc123",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewClickup(
		map[string]interface{}{
			"token": "test",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}
