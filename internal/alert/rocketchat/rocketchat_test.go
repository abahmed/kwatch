package rocketchat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

var testDeps = transport.Dependencies{
	HTTPClient: http.DefaultClient,
	Clock:      clock.RealClock{},
}

func testAppConfig() string {
	return "dev"
}

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewRocketChat(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestRocketChat(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://example.test/hook",
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Rocket Chat")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Nil(c.SendMessage(context.Background(), "test"))
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"webhook": s.URL,
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"webhook": "https://example.test/hook",
	}
	c := NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.webhook = "h ttp://localhost"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	configMap = map[string]interface{}{
		"webhook": "http://localhost:132323",
	}
	c = NewRocketChat(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}
