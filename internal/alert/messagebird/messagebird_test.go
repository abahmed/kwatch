package messagebird

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

	c := NewMessagebird(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestMessagebird(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accessKey": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewMessagebird(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Messagebird")
}

func TestMessagebirdInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewMessagebird(map[string]interface{}{"from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewMessagebird(map[string]interface{}{"accessKey": "a", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewMessagebird(map[string]interface{}{"accessKey": "a", "from": "f"}, &config.App{ClusterName: "dev"})
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
		"accessKey": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewMessagebird(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Equal("AccessKey test", gotAuth)
	assert.Contains(gotBody, `"originator":"kwatch"`)
	assert.Contains(gotBody, `"recipients":["+12025550100"]`)
	assert.Contains(gotBody, `"body":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accessKey": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewMessagebird(configMap, &config.App{ClusterName: "dev"})
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
		"accessKey": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewMessagebird(configMap, &config.App{ClusterName: "dev"})
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
		"accessKey": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewMessagebird(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
