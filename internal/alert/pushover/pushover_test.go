package pushover

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

	c := NewPushover(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestPushover(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token": "test",
		"user":  "user123",
	}
	c := NewPushover(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Pushover")
}

func TestPushoverInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewPushover(
		map[string]interface{}{
			"user": "user123",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewPushover(
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

	var gotCT string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCT = r.Header.Get("Content-Type")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":    "test",
		"user":     "user123",
		"title":    "kwatch",
		"priority": 1,
	}
	c := NewPushover(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "hello alert"))
	assert.Contains(gotCT, "application/x-www-form-urlencoded")
	assert.Contains(gotBody, "token=test")
	assert.Contains(gotBody, "user=user123")
	assert.Contains(gotBody, "message=hello+alert")
	assert.Contains(gotBody, "title=kwatch")
	assert.Contains(gotBody, "priority=1")
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token": "test",
		"user":  "user123",
	}
	c := NewPushover(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "test-pod")
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token": "test",
		"user":  "user123",
	}
	c := NewPushover(configMap, testAppConfig(), testDeps)
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
		"token": "test",
		"user":  "user123",
	}
	c := NewPushover(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestPushoverBuildsForm(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token": "test",
		"user":  "user123",
	}
	c := NewPushover(configMap, testAppConfig(), testDeps)
	c.url = "https://api.pushover.net/1/messages.json"

	assert.True(strings.HasPrefix(c.url, "https://api.pushover.net"))
}
