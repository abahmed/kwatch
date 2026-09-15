package pagerduty

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

func TestPagerdutyEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewPagerDuty(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestPagerduty(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"integrationKey": "testtest",
	}
	c := NewPagerDuty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "PagerDuty")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewPagerDuty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Nil(c.SendMessage(context.Background(), "test"))
}

func TestSendEventResolveActionAndDedupKey(t *testing.T) {
	a := assert.New(t)

	var captured pagerdutyPayload
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&captured)
			w.Write([]byte(`{"isOk": true}`))
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewPagerDuty(configMap, testAppConfig(), testDeps)
	c.url = s.URL
	a.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Action:        "resolved",
		DedupKey:      "incident-hash-12345",
	}
	a.Nil(c.SendEvent(context.Background(), &ev))

	a.Equal("resolve", captured.EventAction, "resolved action must map to 'resolve'")
	a.Equal("incident-hash-12345", captured.DedupKey, "DedupKey must be passed through")
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewPagerDuty(configMap, testAppConfig(), testDeps)
	c.url = s.URL
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

func TestSendEventError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewPagerDuty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = s.URL

	ev := event.Event{
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
	c := NewPagerDuty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "h ttp://localhost"

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

	c = NewPagerDuty(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "http://localhost:132323"

	assert.Error(c.SendEvent(context.Background(), &ev))
}
