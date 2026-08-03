package gitea

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

	c := NewGitea(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestGitea(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Gitea")
	assert.Equal(c.url, "https://gitea.com/api/v1/repos/kwatch/kwatch/issues")
}

func TestGiteaCustomURL(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url":   "https://gitea.example.com/api/v1",
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.url, "https://gitea.example.com/api/v1/repos/kwatch/kwatch/issues")
}

func TestGiteaInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewGitea(map[string]interface{}{"owner": "kwatch", "repo": "kwatch"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewGitea(map[string]interface{}{"token": "test", "repo": "kwatch"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewGitea(map[string]interface{}{"token": "test", "owner": "kwatch"}, &config.App{ClusterName: "dev"})
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
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("test"))
	assert.Equal("token test", gotAuth)
	assert.Contains(gotBody, `"body":"test"`)
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
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, &config.App{ClusterName: "dev"})
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
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, &config.App{ClusterName: "dev"})
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
		"token": "test",
		"owner": "kwatch",
		"repo":  "kwatch",
	}
	c := NewGitea(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
