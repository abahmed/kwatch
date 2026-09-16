package sensugo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/event"
)

var testDeps = transport.Dependencies{
	HTTPClient: http.DefaultClient,
	Clock:      clock.RealClock{},
}

func testAppConfig() string {
	return "dev"
}

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSensugo(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestSensugo(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":    "http://sensu.example.com:8080",
		"apiKey": "test",
	}
	c := NewSensugo(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Sensu Go")
	assert.Equal(c.url, "http://sensu.example.com:8080/api/core/v2/namespaces/default/events")
}

func TestSensugoCustomNamespace(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":       "http://sensu.example.com:8080",
		"apiKey":    "test",
		"namespace": "ops",
		"entity":    "kwatch-agent",
	}
	c := NewSensugo(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "http://sensu.example.com:8080/api/core/v2/namespaces/ops/events")
	assert.Equal(c.entity, "kwatch-agent")
}

func TestSensugoInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSensugo(
		map[string]interface{}{
			"apiKey": "a",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewSensugo(map[string]interface{}{"url": "u"}, testAppConfig(), testDeps)
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
		"url":    "http://sensu.example.com:8080",
		"apiKey": "test",
	}
	c := NewSensugo(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "hello"))
	assert.Equal("Key test", gotAuth)
	assert.Contains(gotBody, `"entity":{"metadata":{"name":"kwatch"}}`)
	assert.Contains(gotBody, `"check":{"metadata":{"name":"kwatch"},"status":1`)
	assert.Contains(gotBody, `"output":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":    "http://sensu.example.com:8080",
		"apiKey": "test",
	}
	c := NewSensugo(configMap, testAppConfig(), testDeps)
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
		"url":    "http://sensu.example.com:8080",
		"apiKey": "test",
	}
	c := NewSensugo(configMap, testAppConfig(), testDeps)
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
		"url":    "http://sensu.example.com:8080",
		"apiKey": "test",
	}
	c := NewSensugo(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
