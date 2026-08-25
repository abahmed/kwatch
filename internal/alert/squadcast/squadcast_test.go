package squadcast

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSquadcast(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSquadcast(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Squadcast")
	assert.Equal(c.url, "https://api.squadcast.com/v2/incidents/api/test")
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
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotBody, `"message"`)
	assert.Contains(gotBody, `"description":"hello"`)
	assert.Contains(gotBody, `"status":"trigger"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			assert.Contains(string(body), "trigger")
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	ev := event.Event{
		PodName:   "test-pod",
		Namespace: "default",
		Reason:    "OOMKILLED",
		Severity:  model.SeverityHigh,
	}
	assert.Nil(c.SendEvent(&ev))
}

func TestInvalidHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"serviceKey": "test",
	}
	c := NewSquadcast(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
