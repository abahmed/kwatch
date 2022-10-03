package matrix

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewMatrix(map[string]string{})
	assert.Nil(c)
}

func TestInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"homeServer": "https://matrix-client.matrix.org",
	}
	c := NewMatrix(config)
	assert.Nil(c)

	config = map[string]string{
		"homeServer":  "https://matrix-client.matrix.org",
		"accessToken": "testToken",
	}
	c = NewMatrix(config)
	assert.Nil(c)

	config = map[string]string{
		"homeServer":     "https://matrix-client.matrix.org",
		"accessToken":    "testToken",
		"internalRoomId": "",
	}
	c = NewMatrix(config)
	assert.Nil(c)

}

func TestMatrix(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"homeServer":     "https://matrix-client.matrix.org",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(config)
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

	config := map[string]string{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(config)
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

	config := map[string]string{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(config)
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

	config := map[string]string{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(config)
	assert.NotNil(c)

	ev := event.Event{
		Name:      "test-pod",
		Container: "test-container",
		Namespace: "default",
		Reason:    "OOMKILLED",
		Logs:      "test\ntestlogs",
		Events: "event1-event2-event3-event1-event2-event3-event1-event2-" +
			"event3\nevent5\nevent6-event8-event11-event12",
	}
	assert.Nil(c.SendEvent(&ev))
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"homeServer":     "h ttp://localhost",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(config)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))

	config = map[string]string{
		"homeServer":     "http://localhost:132323",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c = NewMatrix(config)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
}
