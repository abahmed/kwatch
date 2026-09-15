package ntfy

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

	c := NewNtfy(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestNtfy(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":   "https://ntfy.example.com",
		"topic": "kwatch",
	}
	c := NewNtfy(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Ntfy")
	assert.Equal(c.url, "https://ntfy.example.com/kwatch")
}

func TestNtfyDefaultServer(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"topic": "kwatch",
	}
	c := NewNtfy(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "https://ntfy.sh/kwatch")
}

func TestNtfyInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewNtfy(
		map[string]interface{}{
			"url": "https://ntfy.example.com",
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
		"url":   "https://ntfy.example.com",
		"topic": "kwatch",
		"token": "secret",
	}
	c := NewNtfy(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "test"))
	assert.Equal("Bearer secret", gotAuth)
	assert.Contains(gotBody, `"message":"test"`)
	assert.Contains(gotBody, `"tags"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"topic": "kwatch",
	}
	c := NewNtfy(configMap, testAppConfig(), testDeps)
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
		"topic": "kwatch",
	}
	c := NewNtfy(configMap, testAppConfig(), testDeps)
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
		"topic": "kwatch",
	}
	c := NewNtfy(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
