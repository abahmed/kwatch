package splunkoncall

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

	c := NewSplunkOncall(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSplunkOncall(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Splunk OnCall")
	assert.Equal(c.url, "https://alert.victorops.com/integrations/generic/20131114/alert/everyone/test")
}

func TestSplunkOncallCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":        "https://alert.example.com/integrations/generic/20131114/alert",
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://alert.example.com/integrations/generic/20131114/alert/everyone/test")
}

func TestSplunkOncallInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSplunkOncall(map[string]interface{}{"routingKey": "r"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSplunkOncall(map[string]interface{}{"apiKey": "a"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotBody, `"message_type":"CRITICAL"`)
	assert.Contains(gotBody, `"entity_id":"dev"`)
	assert.Contains(gotBody, `"entity_display_name":"kwatch alert"`)
	assert.Contains(gotBody, `"state_message":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, &config.App{ClusterName: "dev"})
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
		"apiKey":     "test",
		"routingKey": "everyone",
	}
	c := NewSplunkOncall(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
