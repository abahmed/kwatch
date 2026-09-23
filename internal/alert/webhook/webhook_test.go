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
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
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
		"url": "testtest",
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

func TestSendEvent(t *testing.T) {
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

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Error(c.SendEvent(context.Background(), &ev))
}

func TestSendEventError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		"basicAuth": map[string]string{
			"username": "test",
			"password": "test",
		},
	}
	c := NewWebhook(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Nil(c.SendEvent(context.Background(), &ev))
}

func TestSendNotificationUsesStructuredPayload(t *testing.T) {
	var received map[string]interface{}
	s := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer s.Close()

	c := NewWebhook(map[string]interface{}{"url": s.URL}, "dev", testDeps)
	assert.NotNil(t, c)
	notification := &message.Notification{
		DeliveryID: "incident-1:2:create",
		Action:     model.ActionCreate,
		Summary:    message.NotificationSummary{Title: "Pod failed"},
		Diagnostic: message.DiagnosticMetadata{
			Pattern: "internal", Confidence: 0.99,
		},
	}
	assert.NoError(t, c.SendNotification(context.Background(), notification))
	assert.Equal(t, "dev", received["cluster"])
	data := received["notification"].(map[string]interface{})
	assert.Equal(t, "create", data["action"])
	assert.NotContains(t, string(mustJSON(t, data)), "confidence")
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	assert.NoError(t, err)
	return raw
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url": "h ttp://localhost",
	}
	c := NewWebhook(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}

	assert.Error(c.SendEvent(context.Background(), &ev))

	c = NewWebhook(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.webhook = "http://localhost:132323"

	assert.Error(c.SendEvent(context.Background(), &ev))
}
