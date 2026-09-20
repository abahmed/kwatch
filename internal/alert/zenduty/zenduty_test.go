package zenduty

import (
	"context"
	"encoding/json"
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

func TestZendutyEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewZenduty(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestZenduty(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"integrationKey": "testtest",
	}
	c := NewZenduty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Zenduty")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Nil(c.SendMessage(context.Background(), "test"))
}

func TestSendEventCreateIncludesEntityID(t *testing.T) {
	a := assert.New(t)

	var captured zendutyPayload
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&captured)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, testAppConfig(), testDeps)
	c.url = s.URL
	a.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Action:        "create",
		DedupKey:      "entity-456",
	}
	a.Nil(c.SendEvent(context.Background(), &ev))

	a.Equal("entity-456", captured.EntityID, "DedupKey must map to entity_id on create")
}

func TestSendEventResolveSendsResolvedAlertType(t *testing.T) {
	a := assert.New(t)

	var captured zendutyPayload
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&captured)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, testAppConfig(), testDeps)
	c.url = s.URL
	a.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Action:        "resolved",
		DedupKey:      "entity-456",
	}
	a.Nil(c.SendEvent(context.Background(), &ev))

	a.Equal("resolved", captured.AlertType, "resolved action must set alert_type to resolved")
	a.Equal("entity-456", captured.EntityID, "EntityID must be passed on resolve")
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	c.url = s.URL

	ev := event.Event{
		NodeName:      "test-node",
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

func TestSendEventError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	c.url = s.URL

	ev := event.Event{
		NodeName:      "test-node",
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.NotNil(c.SendEvent(context.Background(), &ev))
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "h ttp://localhost"

	ev := event.Event{
		NodeName:      "test-node",
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.NotNil(c.SendEvent(context.Background(), &ev))

	c = NewZenduty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "http://localhost:132323"

	assert.NotNil(c.SendEvent(context.Background(), &ev))
}
