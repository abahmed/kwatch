package opsgenie

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/stretchr/testify/assert"
)

func TestOpsgenieEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewOpsgenie(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestOpsgenie(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey": "testtest",
	}
	c := NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Equal(c.Name(), "Opsgenie")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey": "test",
	}
	c := NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Nil(c.SendMessage("test"))
}

func TestSendEventCreateIncludesAlias(t *testing.T) {
	a := assert.New(t)

	var captured ogPayload
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&captured)
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{}`))
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey": "test",
	}
	c := NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL
	a.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Action:        "create",
		DedupKey:      "alias-123",
	}
	a.Nil(c.SendEvent(&ev))

	a.Equal("alias-123", captured.Alias, "DedupKey must map to alias on create")
}

func TestSendEventResolveCallsCloseEndpoint(t *testing.T) {
	a := assert.New(t)

	closeCalled := false
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			closeCalled = true
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{}`))
		}))
	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey": "test",
	}
	c := NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL
	c.closeURL = s.URL + "/v2/alerts/%s/close?identifierType=alias"
	a.NotNil(c)

	ev := event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Action:        "resolved",
		DedupKey:      "alias-123",
	}
	a.Nil(c.SendEvent(&ev))
	a.True(closeCalled, "resolved action must call close endpoint")
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey": "test",
	}
	c := NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
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
	assert.Nil(c.SendEvent(&ev))
}

func TestSendEventError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey": "test",
	}
	c := NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
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
	assert.NotNil(c.SendEvent(&ev))
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey": "test",
	}
	c := NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
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
	assert.NotNil(c.SendEvent(&ev))

	c = NewOpsgenie(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = "http://localhost:132323"

	assert.NotNil(c.SendEvent(&ev))
}
