package zulip

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
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

	c := NewZulip(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestZulip(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":     "https://chat.corp.io",
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.Name(), "Zulip")
	assert.Equal(c.url, "https://chat.corp.io/api/v1/messages")
}

func TestZulipCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":     "https://zulip.corp.io",
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	assert.Equal(c.url, "https://zulip.corp.io/api/v1/messages")
}

func TestZulipRequiresURL(t *testing.T) {
	c := NewZulip(map[string]interface{}{
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}, testAppConfig(), testDeps)
	assert.Nil(t, c)
}

func TestZulipRejectsExampleHosts(t *testing.T) {
	for _, server := range []string{
		"https://zulip.example.com", "https://example.org",
		"http://chat.example.net", "https://zulip.example",
	} {
		c := NewZulip(map[string]interface{}{
			"url":     server,
			"email":   "kwatch@example.com",
			"token":   "test",
			"channel": "alerts",
		}, testAppConfig(), testDeps)
		assert.Nil(t, c, server)
	}
}

func TestZulipInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewZulip(
		map[string]interface{}{
			"token":   "test",
			"channel": "alerts",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewZulip(
		map[string]interface{}{
			"email":   "kwatch@example.com",
			"channel": "alerts",
		},
		testAppConfig(),
		testDeps,
	)
	assert.Nil(c)

	c = NewZulip(
		map[string]interface{}{
			"email": "kwatch@example.com",
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
			w.Write([]byte(`{"result":"success"}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":     "https://zulip.corp.io",
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.Nil(c.SendMessage(context.Background(), "hello"))
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("kwatch@example.com:test"))
	assert.Equal(expectedAuth, gotAuth)
	assert.Contains(gotBody, "type=stream")
	assert.Contains(gotBody, "to=alerts")
	assert.Contains(gotBody, "content=hello")
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":     "https://zulip.corp.io",
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, testAppConfig(), testDeps)
	c.url = s.URL

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":     "https://zulip.corp.io",
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, testAppConfig(), testDeps)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
