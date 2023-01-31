package rocketchat

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

	c := NewRocketChat(map[string]string{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestRocketChat(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]string{
		"webhook": "testtest",
	}
	c := NewRocketChat(configMap, &config.App{ClusterName: "dev"})
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

	configMap := map[string]string{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, &config.App{ClusterName: "dev"})
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

	configMap := map[string]string{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, &config.App{ClusterName: "dev"})
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

	configMap := map[string]string{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, &config.App{ClusterName: "dev"})
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

	configMap := map[string]string{
		"webhook": "h ttp://localhost",
	}
	c := NewRocketChat(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))

	configMap = map[string]string{
		"webhook": "http://localhost:132323",
	}
	c = NewRocketChat(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.NotNil(c.SendMessage("test"))
}
