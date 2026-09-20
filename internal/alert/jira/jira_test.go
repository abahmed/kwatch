package jira

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
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
			"url":        "u",
			"apiToken":   "t",
			"projectKey": "P",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewJira(
		map[string]interface{}{
			"url":        "u",
			"user":       "u",
			"projectKey": "P",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewJira(
		map[string]interface{}{
			"url":      "u",
			"user":     "u",
			"apiToken": "t",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusCreated)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":        "https://kwatch.atlassian.net",
		"user":       "ops@example.com",
		"apiToken":   "test",
		"projectKey": "OPS",
	}
	c := NewJira(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "hello"))
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("ops@example.com:test"))
	assert.Equal(expectedAuth, gotAuth)
	assert.Contains(gotBody, `"project":{"key":"OPS"}`)
	assert.Contains(gotBody, `"issuetype":{"name":"Task"}`)
	assert.Contains(gotBody, `"summary":"kwatch alert: dev"`)
	assert.Contains(gotBody, `"description":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":        "https://kwatch.atlassian.net",
		"user":       "ops@example.com",
		"apiToken":   "test",
		"projectKey": "OPS",
	}
	c := NewJira(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.WriteHeader(http.StatusCreated)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":        "https://kwatch.atlassian.net",
		"user":       "ops@example.com",
		"apiToken":   "test",
		"projectKey": "OPS",
	}
	c := NewJira(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	ev := event.Event{
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "OOMKILLED",
	}
	assert.Nil(c.SendEvent(context.Background(), &ev))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":        "https://kwatch.atlassian.net",
		"user":       "ops@example.com",
		"apiToken":   "test",
		"projectKey": "OPS",
	}
	c := NewJira(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
