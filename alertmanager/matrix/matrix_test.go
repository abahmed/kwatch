package matrix

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewMatrix(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"homeServer": "https://matrix-client.matrix.org",
	}
	c := NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	configMap = map[string]interface{}{
		"homeServer":  "https://matrix-client.matrix.org",
		"accessToken": "testToken",
	}
	c = NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	configMap = map[string]interface{}{
		"homeServer":     "https://matrix-client.matrix.org",
		"accessToken":    "testToken",
		"internalRoomId": "",
	}
	c = NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.Nil(c)

}

func TestMatrix(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"homeServer":     "https://matrix-client.matrix.org",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Equal(c.Name(), "Matrix")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Nil(c.SendMessage("test"))
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, &config.App{ClusterName: "dev"})
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
	assert.Nil(c.SendEvent(&ev))
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"homeServer":     "h ttp://localhost",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))

	configMap = map[string]interface{}{
		"homeServer":     "http://localhost:132323",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c = NewMatrix(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
}
