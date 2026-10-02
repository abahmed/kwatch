package github

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

	c := NewGithub(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestGithub(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGithub(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Github")
	assert.Equal(c.url, "https://api.github.com/repos/kwatch/kwatch/issues")
}

func TestGithubInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGithub(
		map[string]interface{}{
			"owner": "kwatch",
			"repo":  "kwatch",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewGithub(
		map[string]interface{}{
			"token": "test",
			"repo":  "kwatch",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewGithub(
		map[string]interface{}{
			"token": "test",
			"owner": "kwatch",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}
