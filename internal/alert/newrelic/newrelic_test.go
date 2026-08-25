package newrelic

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

	c := NewNewRelic(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestNewRelic(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"accountId": "123456",
	}
	c := NewNewRelic(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "New Relic")
	assert.Equal(c.url, "https://insights-collector.newrelic.com/v1/accounts/123456/events")
}

func TestNewRelicInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewNewRelic(map[string]interface{}{"accountId": "123456"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewNewRelic(map[string]interface{}{"apiKey": "test"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotKey string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey = r.Header.Get("Api-Key")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"accountId": "123456",
	}
	c := NewNewRelic(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Equal("test", gotKey)
	assert.Contains(gotBody, `"eventType":"KwatchAlert"`)
	assert.Contains(gotBody, `"message":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"accountId": "123456",
	}
	c := NewNewRelic(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"accountId": "123456",
	}
	c := NewNewRelic(configMap, &config.App{ClusterName: "dev"})
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
		"apiKey":    "test",
		"accountId": "123456",
	}
	c := NewNewRelic(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
