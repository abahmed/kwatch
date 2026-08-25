package twilio

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

	c := NewTwilio(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestTwilio(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"accountSid": "AC123",
		"authToken":  "test",
		"from":       "+12025550199",
		"to":         "+12025550100",
	}
	c := NewTwilio(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Twilio")
	assert.Equal(c.url, "https://api.twilio.com/2010-04-01/Accounts/AC123/Messages.json")
}

func TestTwilioInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewTwilio(map[string]interface{}{"authToken": "t", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewTwilio(map[string]interface{}{"accountSid": "a", "from": "f", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewTwilio(map[string]interface{}{"accountSid": "a", "authToken": "t", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewTwilio(map[string]interface{}{"accountSid": "a", "authToken": "t", "from": "f"}, &config.App{ClusterName: "dev"})
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
		"accountSid": "AC123",
		"authToken":  "test",
		"from":       "+12025550199",
		"to":         "+12025550100",
	}
	c := NewTwilio(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("AC123:test"))
	assert.Equal(expectedAuth, gotAuth)
	assert.Contains(gotBody, "From=%2B12025550199")
	assert.Contains(gotBody, "To=%2B12025550100")
	assert.Contains(gotBody, "Body=hello")
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"accountSid": "AC123",
		"authToken":  "test",
		"from":       "+12025550199",
		"to":         "+12025550100",
	}
	c := NewTwilio(configMap, &config.App{ClusterName: "dev"})
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
		"accountSid": "AC123",
		"authToken":  "test",
		"from":       "+12025550199",
		"to":         "+12025550100",
	}
	c := NewTwilio(configMap, &config.App{ClusterName: "dev"})
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
		"accountSid": "AC123",
		"authToken":  "test",
		"from":       "+12025550199",
		"to":         "+12025550100",
	}
	c := NewTwilio(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
