package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
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

	c := NewWebhook(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestWebhook(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url": "https://example.test/hook",
		"headers": []interface{}{
			map[string]string{
				"name":  "test",
				"value": "test",
			},
		},
	}
	c := NewWebhook(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Webhook")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)
	var received map[string]string

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NoError(json.NewDecoder(r.Body).Decode(&received))
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url": s.URL,
		"headers": []interface{}{
			map[string]string{
				"name":  "test",
				"value": "test",
			},
		},
	}
	c := NewWebhook(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Nil(c.SendMessage(context.Background(), "test"))
	assert.Equal("dev", received["Cluster"])
	assert.Equal("test", received["Message"])
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url": s.URL,
	}
	c := NewWebhook(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Error(c.SendMessage(context.Background(), "test"))
}

func TestInvalidHTTPRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url": "https://example.test/hook",
	}
	c := NewWebhook(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.webhook = "h ttp://localhost"
	m := notification.Message{Key: "k", Title: "api failed"}
	assert.Error(c.SendIncident(context.Background(), m))

	c.webhook = "http://localhost:132323"
	assert.Error(c.SendIncident(context.Background(), m))
}
