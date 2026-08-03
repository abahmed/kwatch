package vonage

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

	c := NewVonage(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestVonage(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"apiSecret": "secret",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewVonage(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Vonage")
}

func TestVonageInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewVonage(map[string]interface{}{"apiSecret": "s", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewVonage(map[string]interface{}{"apiKey": "a", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewVonage(map[string]interface{}{"apiKey": "a", "apiSecret": "s", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewVonage(map[string]interface{}{"apiKey": "a", "apiSecret": "s", "from": "f"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte(`{"messages":[{"status":"0"}]}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"apiSecret": "secret",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewVonage(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotBody, "api_key=test")
	assert.Contains(gotBody, "from=kwatch")
	assert.Contains(gotBody, "text=hello")
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"apiSecret": "secret",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewVonage(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte(`{"messages":[{"status":"0"}]}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey":    "test",
		"apiSecret": "secret",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewVonage(configMap, &config.App{ClusterName: "dev"})
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
		"apiSecret": "secret",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewVonage(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
