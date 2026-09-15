package clickup

import (
	"context"
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

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":    "test",
		"listId":   "abc123",
		"priority": 2,
	}
	c := NewClickup(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "hello"))
	assert.Equal("test", gotAuth)
	assert.Contains(gotBody, `"name"`)
	assert.Contains(gotBody, `"description":"hello"`)
	assert.Contains(gotBody, `"priority":2`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":  "test",
		"listId": "abc123",
	}
	c := NewClickup(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":  "test",
		"listId": "abc123",
	}
	c := NewClickup(configMap, testAppConfig(), testDeps)
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
		"token":  "test",
		"listId": "abc123",
	}
	c := NewClickup(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
