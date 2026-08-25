package webex

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewWebex(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestWebex(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "test",
		"roomId":      "room123",
	}
	c := NewWebex(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Webex")
}

func TestWebexToPersonEmail(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken":   "test",
		"toPersonEmail": "ops@example.com",
	}
	c := NewWebex(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
}

func TestWebexInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewWebex(map[string]interface{}{"roomId": "room123"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewWebex(map[string]interface{}{"accessToken": "test"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "test",
		"roomId":      "room123",
	}
	c := NewWebex(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("test"))
	assert.Equal("Bearer test", gotAuth)
	assert.Contains(gotBody, `"roomId":"room123"`)
	assert.Contains(gotBody, `"markdown":"test"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken":   "test",
		"toPersonEmail": "ops@example.com",
	}
	c := NewWebex(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessToken": "test",
		"roomId":      "room123",
	}
	c := NewWebex(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	ev := event.Event{
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "OOMKILLED",
	}
	assert.Nil(c.SendEvent(&ev))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessToken": "test",
		"roomId":      "room123",
	}
	c := NewWebex(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
