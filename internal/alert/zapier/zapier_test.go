package zapier

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

	c := NewZapier(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestZapier(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"url": "https://hooks.zapier.com/hooks/catch/123/abc",
	}
	c := NewZapier(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Zapier")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url": "https://hooks.zapier.com/hooks/catch/123/abc",
	}
	c := NewZapier(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("test"))
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
		"url": "https://hooks.zapier.com/hooks/catch/123/abc",
	}
	c := NewZapier(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte(`{}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"url": "https://hooks.zapier.com/hooks/catch/123/abc",
	}
	c := NewZapier(configMap, &config.App{ClusterName: "dev"})
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
		"url": "https://hooks.zapier.com/hooks/catch/123/abc",
	}
	c := NewZapier(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
