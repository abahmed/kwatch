package alerta

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

	c := NewAlerta(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestAlerta(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":    "https://alerta.example.com",
		"apiKey": "test",
	}
	c := NewAlerta(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Alerta")
	assert.Equal(c.url, "https://alerta.example.com/api/alert")
	assert.Equal(c.environment, "Production")
	assert.Equal(c.service, "kwatch")
}

func TestAlertaInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewAlerta(map[string]interface{}{"apiKey": "a"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewAlerta(map[string]interface{}{"url": "u"}, &config.App{ClusterName: "dev"})
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
			w.WriteHeader(http.StatusCreated)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":    "https://alerta.example.com",
		"apiKey": "test",
	}
	c := NewAlerta(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Equal("Key test", gotAuth)
	assert.Contains(gotBody, `"resource":"kwatch/dev"`)
	assert.Contains(gotBody, `"event":"kwatch"`)
	assert.Contains(gotBody, `"environment":"Production"`)
	assert.Contains(gotBody, `"severity":"critical"`)
	assert.Contains(gotBody, `"service":["kwatch"]`)
	assert.Contains(gotBody, `"text":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":    "https://alerta.example.com",
		"apiKey": "test",
	}
	c := NewAlerta(configMap, &config.App{ClusterName: "dev"})
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
		"url":    "https://alerta.example.com",
		"apiKey": "test",
	}
	c := NewAlerta(configMap, &config.App{ClusterName: "dev"})
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
		"url":    "https://alerta.example.com",
		"apiKey": "test",
	}
	c := NewAlerta(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
