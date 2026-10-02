package gitlab

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

	c := NewGitlab(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestGitlab(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":     "test",
		"projectId": "1",
	}
	c := NewGitlab(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Gitlab")
	assert.Equal(c.url, "https://gitlab.com/api/v4/projects/1/issues")
}

func TestGitlabCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":       "https://gitlab.example.com/api/v4",
		"token":     "test",
		"projectId": "1",
	}
	c := NewGitlab(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "https://gitlab.example.com/api/v4/projects/1/issues")
}

func TestGitlabInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGitlab(
		map[string]interface{}{
			"projectId": "1",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewGitlab(
		map[string]interface{}{
			"token": "test",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}
