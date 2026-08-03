package signal

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

	c := NewSignal(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSignal(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"number": "+12025550199",
		"to":     "+12025550100",
	}
	c := NewSignal(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Signal")
	assert.Equal(c.url, "http://localhost:8080/v2/send")
}

func TestSignalCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":    "https://signal.example.com:8081",
		"number": "+12025550199",
		"to":     "+12025550100",
	}
	c := NewSignal(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://signal.example.com:8081/v2/send")
}

func TestSignalInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSignal(map[string]interface{}{"to": "+12025550100"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSignal(map[string]interface{}{"number": "+12025550199"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusCreated)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"number": "+12025550199",
		"to":     "+12025550100",
	}
	c := NewSignal(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotBody, `"message":"hello"`)
	assert.Contains(gotBody, `"number":"+12025550199"`)
	assert.Contains(gotBody, `"recipients":["+12025550100"]`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"number": "+12025550199",
		"to":     "+12025550100",
	}
	c := NewSignal(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.WriteHeader(http.StatusCreated)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"number": "+12025550199",
		"to":     "+12025550100",
	}
	c := NewSignal(configMap, &config.App{ClusterName: "dev"})
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
		"number": "+12025550199",
		"to":     "+12025550100",
	}
	c := NewSignal(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
