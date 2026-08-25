package threema

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

	c := NewThreema(map[string]interface{}{}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestThreema(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"gatewayId": "*TESTGATEWAY",
		"secret":    "test",
		"to":        "TEST1234",
	}
	c := NewThreema(configMap, &config.App{ClusterName: "dev"})
	assert.NotNil(c)
	assert.Equal(c.Name(), "Threema")
	assert.Equal(c.url, "https://gateway.threema.ch/push_simple")
}

func TestThreemaInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewThreema(map[string]interface{}{"secret": "s", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewThreema(map[string]interface{}{"gatewayId": "g", "to": "t"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)

	c = NewThreema(map[string]interface{}{"gatewayId": "g", "secret": "s"}, &config.App{ClusterName: "dev"})
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	var gotBody string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.Write([]byte("OK"))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"gatewayId": "*TESTGATEWAY",
		"secret":    "test",
		"to":        "TEST1234",
	}
	c := NewThreema(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.Nil(c.SendMessage("hello"))
	assert.Contains(gotBody, "from=%2ATESTGATEWAY")
	assert.Contains(gotBody, "to=TEST1234")
	assert.Contains(gotBody, "secret=test")
	assert.Contains(gotBody, "text=hello")
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"gatewayId": "*TESTGATEWAY",
		"secret":    "test",
		"to":        "TEST1234",
	}
	c := NewThreema(configMap, &config.App{ClusterName: "dev"})
	c.url = s.URL

	assert.NotNil(c.SendMessage("test"))
}

func TestSendEvent(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(string(body), "OOMKILLED")
			w.Write([]byte("OK"))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"gatewayId": "*TESTGATEWAY",
		"secret":    "test",
		"to":        "TEST1234",
	}
	c := NewThreema(configMap, &config.App{ClusterName: "dev"})
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
		"gatewayId": "*TESTGATEWAY",
		"secret":    "test",
		"to":        "TEST1234",
	}
	c := NewThreema(configMap, &config.App{ClusterName: "dev"})
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage("test"))

	c.url = "http://localhost:132323/%s"
	assert.NotNil(c.SendMessage("test"))
}
