package gotify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGotify(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestGotify(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":   "https://gotify.example.com",
		"token": "test",
	}
	c := NewGotify(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Gotify")
	assert.Equal(c.url, "https://gotify.example.com/message")
}

func TestGotifyInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGotify(map[string]interface{}{"url": "https://gotify.example.com"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewGotify(map[string]interface{}{"token": "test"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotKey string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey = r.Header.Get("X-Gotify-Key")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":   "https://gotify.example.com",
		"token": "test",
	}
	c := NewGotify(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("test"))
	assert.Equal("test", gotKey)
	assert.Contains(gotBody, `"message":"test"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":   "https://gotify.example.com",
		"token": "test",
	}
	c := NewGotify(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "test-pod")
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url":   "https://gotify.example.com",
		"token": "test",
	}
	c := NewGotify(configMap, &config.App{ClusterName: "dev"})
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
		"url":   "https://gotify.example.com",
		"token": "test",
	}
	c := NewGotify(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}

func TestGotifyTitlePriority(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":      "https://gotify.example.com",
		"token":    "test",
		"title":    "kwatch",
		"priority": 5,
	}
	c := NewGotify(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal("kwatch", c.title)
	assert.Equal(5, c.priority)

	assert.True(strings.HasPrefix(c.url, "https://gotify.example.com"))
}
