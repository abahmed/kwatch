package dingtalk

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
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

func TestNewDingTalkWithTitle(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
		"title":       "Custom Title",
		"secret":      "secret123",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal("Custom Title", c.title)
	assert.Equal("secret123", c.secret)
}

func TestSendEventWithDefaultTitle(t *testing.T) {
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

	ev := &event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test logs",
	}
	err := c.SendEvent(ev)
	assert.Nil(err)
}

func TestSendMessageWithSecret(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
		"secret":      "testSecret123",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	err := c.SendMessage("test message with secret")
	assert.Nil(err)
}

func TestSendEventWithSecret(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
		"secret":      "testSecret456",
		"title":       "Custom Event Title",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	ev := &event.Event{
		PodName:       "event-pod",
		ContainerName: "event-container",
		Namespace:     "event-ns",
		Reason:        "CrashLoopBackOff",
		Logs:          "crash logs",
	}
	err := c.SendEvent(ev)
	assert.Nil(err)
}

func TestSendMessageJsonMarshalError(t *testing.T) {
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

	ev := &event.Event{
		PodName: "test",
	}
	err := c.SendEvent(ev)
	assert.Nil(err)
}

func TestSendAPIResponseReadError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1000000")
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	err := c.SendMessage("test")
	assert.NotNil(err)
}

func TestComputeHmacSha256(t *testing.T) {
	assert := assert.New(t)

	result := computeHmacSha256("message", "secret")
	assert.NotEmpty(result)
	assert.NotEqual("message", result)
}

func TestGetSignature(t *testing.T) {
	assert := assert.New(t)

	result := getSignature("testSecret")
	assert.Contains(result, "timestamp=")
	assert.Contains(result, "sign=")
}

func TestSendMessageInvalidJsonResponse(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{invalid json`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	err := c.SendMessage("test")
	assert.NotNil(err)
}

func TestSendEventEmptyTitle(t *testing.T) {
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
	c.title = ""

	ev := &event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test logs",
	}
	err := c.SendEvent(ev)
	assert.Nil(err)
}

func TestSendMessageNetworkError(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = "http://localhost:99999/send"

	err := c.SendMessage("test")
	assert.NotNil(err)
}

func TestSendEventNetworkError(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = "http://localhost:99999/send"

	ev := &event.Event{
		PodName:       "test-pod",
		ContainerName: "test-container",
		Namespace:     "default",
		Reason:        "OOMKILLED",
		Logs:          "test logs",
	}
	err := c.SendEvent(ev)
	assert.NotNil(err)
}

func TestSendMessageErrorResponseStatus(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"errcode": 400, "errmsg": "bad request"}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = s.URL + "/send?accessToken=%s"

	err := c.SendMessage("test")
	assert.NotNil(err)
}

func TestSendMessageWithInvalidUTF8(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "testToken",
	}
	c := NewDingTalk(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	c.url = "http://localhost:99999"

	invalidUTF8 := string([]byte{0xff, 0xfe})
	err := c.SendMessage(invalidUTF8)
	assert.NotNil(err)
}
