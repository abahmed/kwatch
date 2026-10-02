package gitea

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

	c := NewGitea(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestGitea(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Gitea")
	assert.Equal(c.url, "https://gitea.com/api/v1/repos/kwatch/kwatch/issues")
}

func TestGiteaCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":   "https://gitea.example.com/api/v1",
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "https://gitea.example.com/api/v1/repos/kwatch/kwatch/issues")
}

func TestGiteaInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGitea(
		map[string]interface{}{
			"owner": "kwatch",
			"repo":  "kwatch",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewGitea(
		map[string]interface{}{
			"token": "test",
			"repo":  "kwatch",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewGitea(
		map[string]interface{}{
			"token": "test",
			"owner": "kwatch",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}
