package dingtalk

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

	c := NewDingTalk(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestDingTalk(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)

	assert.Equal(c.Name(), "DingTalk")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
		"secret":      "secret1",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	assert.Nil(c.SendMessage("test"))
}

func TestSendMessageInvalidBody(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1")
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	assert.NotNil(c.SendMessage("test"))
}

func TestSendMessageInvalidJson(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	assert.NotNil(c.SendMessage("test"))
}

func TestSendMessageErrorResponse(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"errcode": 1234, "errmsg": "error"}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

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
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

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
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = "h ttp://localhost" + "/send?accessToken=%s"

	assert.NotNil(c.SendMessage("test"))

	configMap = map[string]interface{}{
		"accessToken": "testToken",
	}
	c = NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = "http://localhost:132323" + "/send?accessToken=%s"

	assert.NotNil(c.SendMessage("test"))
}
