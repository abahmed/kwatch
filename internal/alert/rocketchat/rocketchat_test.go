package rocketchat

import (
	"context"
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

	c := NewRocketChat(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestRocketChat(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "testtest",
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Rocket Chat")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Nil(c.SendMessage(context.Background(), "test"))
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
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

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "h ttp://localhost",
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}
	c = NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
