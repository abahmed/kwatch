package zulip

import (
	"encoding/base64"
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

	c := NewZulip(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestZulip(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Zulip")
	assert.Equal(c.url, "https://api.zulip.com/api/v1/messages")
}

func TestZulipCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":     "https://zulip.example.com",
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://zulip.example.com/api/v1/messages")
}

func TestZulipInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewZulip(map[string]interface{}{"token": "test", "channel": "alerts"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewZulip(map[string]interface{}{"email": "kwatch@example.com", "channel": "alerts"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewZulip(map[string]interface{}{"email": "kwatch@example.com", "token": "test"}, &config.App{ClusterName: "dev"})
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
			w.Write([]byte(`{"result":"success"}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("kwatch@example.com:test"))
	assert.Equal(expectedAuth, gotAuth)
	assert.Contains(gotBody, "type=stream")
	assert.Contains(gotBody, "to=alerts")
	assert.Contains(gotBody, "content=hello")
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte(`{"result":"success"}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, &config.App{ClusterName: "dev"})
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
		"email":   "kwatch@example.com",
		"token":   "test",
		"channel": "alerts",
	}
	c := NewZulip(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
