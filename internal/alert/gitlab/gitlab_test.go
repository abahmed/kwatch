package gitlab

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

	c := NewGitlab(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestGitlab(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":     "test",
		"projectId": "1",
	}
	c := NewGitlab(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Gitlab")
	assert.Equal(c.url, "https://gitlab.com/api/v4/projects/1/issues")
}

func TestGitlabCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":       "https://gitlab.example.com/api/v4",
		"token":     "test",
		"projectId": "1",
	}
	c := NewGitlab(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://gitlab.example.com/api/v4/projects/1/issues")
}

func TestGitlabInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGitlab(map[string]interface{}{"projectId": "1"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewGitlab(map[string]interface{}{"token": "test"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotAuth string
	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("PRIVATE-TOKEN")
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusCreated)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":     "test",
		"projectId": "1",
	}
	c := NewGitlab(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("test"))
	assert.Equal("test", gotAuth)
	assert.Contains(gotBody, `"description":"test"`)
	assert.Contains(gotBody, `"title"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":     "test",
		"projectId": "1",
	}
	c := NewGitlab(configMap, &config.App{ClusterName: "dev"})
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
		"token":     "test",
		"projectId": "1",
	}
	c := NewGitlab(configMap, &config.App{ClusterName: "dev"})
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
		"projectId": "1",
	}
	c := NewGitlab(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
