package plivo

import (
	"encoding/base64"
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

	c := NewPlivo(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestPlivo(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"authId":    "MA123",
		"authToken": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewPlivo(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Plivo")
	assert.Equal(c.url, "https://api.plivo.com/v1/Account/MA123/Message/")
}

func TestPlivoInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewPlivo(map[string]interface{}{"authToken": "t", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewPlivo(map[string]interface{}{"authId": "a", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewPlivo(map[string]interface{}{"authId": "a", "authToken": "t", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewPlivo(map[string]interface{}{"authId": "a", "authToken": "t", "from": "f"}, &config.App{ClusterName: "dev"})
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
		"authId":    "MA123",
		"authToken": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewPlivo(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("MA123:test"))
	assert.Equal(expectedAuth, gotAuth)
	assert.Contains(gotBody, "src=kwatch")
	assert.Contains(gotBody, "dst=%2B12025550100")
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
		"authId":    "MA123",
		"authToken": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewPlivo(configMap, &config.App{ClusterName: "dev"})
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
		"authId":    "MA123",
		"authToken": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewPlivo(configMap, &config.App{ClusterName: "dev"})
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
		"authId":    "MA123",
		"authToken": "test",
		"from":      "kwatch",
		"to":        "+12025550100",
	}
	c := NewPlivo(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
