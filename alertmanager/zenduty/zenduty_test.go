package zenduty

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func TestZendutyEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewZenduty(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestZenduty(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"integrationKey": "testtest",
	}
	c := NewZenduty(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Equal(c.Name(), "Zenduty")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Nil(c.SendMessage("test"))
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
	c := NewZenduty(configMap, &config.App{ClusterName: "dev"})
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
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, &config.App{ClusterName: "dev"})
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
		"integrationKey": "test",
	}
	c := NewZenduty(configMap, &config.App{ClusterName: "dev"})
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

	c = NewZenduty(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = "http://localhost:132323"

	assert.NotNil(c.SendEvent(&ev))
}
