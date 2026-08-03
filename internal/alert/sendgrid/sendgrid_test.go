package sendgrid

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

	c := NewSendgrid(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendgrid(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"apiKey": "test",
		"from":   "kwatch@example.com",
		"to":     []interface{}{"ops@example.com"},
	}
	c := NewSendgrid(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Sendgrid")
	assert.Equal(c.url, "https://api.sendgrid.com/v3/mail/send")
}

func TestSendgridInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewSendgrid(map[string]interface{}{"from": "f", "to": []interface{}{"t"}}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSendgrid(map[string]interface{}{"apiKey": "a", "to": []interface{}{"t"}}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewSendgrid(map[string]interface{}{"apiKey": "a", "from": "f"}, &config.App{ClusterName: "dev"})
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
		"apiKey":  "test",
		"from":    "kwatch@example.com",
		"to":      []interface{}{"ops@example.com", "dev@example.com"},
		"subject": "kwatch alert",
	}
	c := NewSendgrid(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Equal("Bearer test", gotAuth)
	assert.Contains(gotBody, `"email":"ops@example.com"`)
	assert.Contains(gotBody, `"email":"dev@example.com"`)
	assert.Contains(gotBody, `"subject":"kwatch alert"`)
	assert.Contains(gotBody, `"value":"hello"`)
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"apiKey": "test",
		"from":   "kwatch@example.com",
		"to":     []interface{}{"ops@example.com"},
	}
	c := NewSendgrid(configMap, &config.App{ClusterName: "dev"})
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
		"apiKey": "test",
		"from":   "kwatch@example.com",
		"to":     []interface{}{"ops@example.com"},
	}
	c := NewSendgrid(configMap, &config.App{ClusterName: "dev"})
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
		"apiKey": "test",
		"from":   "kwatch@example.com",
		"to":     []interface{}{"ops@example.com"},
	}
	c := NewSendgrid(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
