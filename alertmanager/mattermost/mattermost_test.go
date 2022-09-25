package mattermost

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/event"
	"github.com/stretchr/testify/assert"
)

func TestMattermostEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewMattermost(map[string]string{})
	assert.Nil(c)
}

func TestMattermost(t *testing.T) {
	assert := assert.New(t)

	config := map[string]string{
		"webhook": "testtest",
	}
	c := NewMattermost(config)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Mattermost")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"isOk": true}`))
	}))

	defer s.Close()

	config := map[string]string{
		"webhook": s.URL,
	}
	c := NewMattermost(config)
	assert.NotNil(c)

	assert.Nil(c.SendMessage("test"))
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))

	defer s.Close()

	config := map[string]string{
		"webhook": s.URL,
	}
	c := NewMattermost(config)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"isOk": true}`))
	}))

	defer s.Close()

	config := map[string]string{
		"webhook": s.URL,
	}
	c := NewMattermost(config)
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
