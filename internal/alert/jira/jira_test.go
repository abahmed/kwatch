package jira

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

	c := NewJira(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestJira(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":        "https://kwatch.atlassian.net",
		"user":       "ops@example.com",
		"apiToken":   "test",
		"projectKey": "OPS",
	}
	c := NewJira(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Jira")
	assert.Equal(c.url, "https://kwatch.atlassian.net/rest/api/2/issue")
	assert.Equal(c.issueType, "Task")
}

func TestJiraInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewJira(
		map[string]interface{}{
			"user":       "u",
			"apiToken":   "t",
			"projectKey": "P",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewJira(
		map[string]interface{}{
			"url":        "https://jira.example.test",
			"apiToken":   "t",
			"projectKey": "P",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewJira(
		map[string]interface{}{
			"url":        "https://jira.example.test",
			"user":       "u",
			"projectKey": "P",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewJira(
		map[string]interface{}{
			"url":      "https://jira.example.test",
			"user":     "u",
			"apiToken": "t",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}
