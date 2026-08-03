package goalert

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

	c := NewGoalert(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestGoalert(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":     "test",
		"serviceId": "SVC123",
	}
	c := NewGoalert(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "GoAlert")
	assert.Equal(c.url, "https://goalert.example.com/api/v2/events")
}

func TestGoalertCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":       "https://goalert.example.org",
		"token":     "test",
		"serviceId": "SVC123",
	}
	c := NewGoalert(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://goalert.example.org/api/v2/events")
}

func TestGoalertInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGoalert(map[string]interface{}{"serviceId": "S"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewGoalert(map[string]interface{}{"token": "t"}, &config.App{ClusterName: "dev"})
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
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":     "test",
		"serviceId": "SVC123",
	}
	c := NewGoalert(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Equal("Bearer test", gotAuth)
	assert.Contains(gotBody, `"type":"incident.create"`)
	assert.Contains(gotBody, `"serviceID":"SVC123"`)
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
		"token":     "test",
		"serviceId": "SVC123",
	}
	c := NewGoalert(configMap, &config.App{ClusterName: "dev"})
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
		"token":     "test",
		"serviceId": "SVC123",
	}
	c := NewGoalert(configMap, &config.App{ClusterName: "dev"})
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
		"token":     "test",
		"serviceId": "SVC123",
	}
	c := NewGoalert(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
