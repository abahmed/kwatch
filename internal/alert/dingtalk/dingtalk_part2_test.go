package dingtalk

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

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
