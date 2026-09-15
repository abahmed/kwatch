package ifttt

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

	c := NewIfttt(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestIfttt(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"key": "abc123",
	}
	c := NewIfttt(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Ifttt")
	assert.Equal(c.url, "https://maker.ifttt.com/trigger/kwatch/with/key/abc123")
}

func TestIftttCustomEvent(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"key":   "abc123",
		"event": "k8s-alert",
	}
	c := NewIfttt(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "https://maker.ifttt.com/trigger/k8s-alert/with/key/abc123")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"key": "abc123",
	}
	c := NewIfttt(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "test"))
	assert.Contains(gotBody, `"value1":"kwatch"`)
	assert.Contains(gotBody, `"value2":"test"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"key": "abc123",
	}
	c := NewIfttt(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"key": "abc123",
	}
	c := NewIfttt(configMap, testAppConfig(), testDeps)
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
		"key": "abc123",
	}
	c := NewIfttt(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
