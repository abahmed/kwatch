package telegram

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

	c := NewTelegram(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestTelegram(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":  "testtest",
		"chatId": "tessst",
	}
	c := NewTelegram(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Telegram")
}

func TestTelegramInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token": "test",
	}
	c := NewTelegram(configMap, testAppConfig(), testDeps)
	assert.Nil(c)

	configMap = map[string]interface{}{
		"chatId": "test",
	}
	c = NewTelegram(configMap, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":  "test",
		"chatId": "test",
	}
	c := NewTelegram(configMap, testAppConfig(), testDeps)
	c.url = s.URL + "/%s"
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
		"token":  "test",
		"chatId": "test",
	}
	c := NewTelegram(configMap, testAppConfig(), testDeps)
	c.url = s.URL + "/%s"
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"token":  "test",
		"chatId": "test",
	}

	c := NewTelegram(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "h ttp://localhost/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	c = NewTelegram(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.url = "http://localhost:132323/%s"

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestMaskString(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("****", maskString("abc"))
	assert.Equal("****", maskString("ab"))
	assert.Equal("****", maskString("a"))
	assert.Equal("test***", maskString("test123"))
	assert.Equal("long*****", maskString("longvalue"))
}

func TestSendMessageStatusAccepted(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":  "test",
		"chatId": "test",
	}
	c := NewTelegram(configMap, testAppConfig(), testDeps)
	c.url = s.URL + "/%s"

	err := c.SendMessage(context.Background(), "test")
	assert.Nil(err)
}

func TestSendMessageStatusOK(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"token":  "test",
		"chatId": "test",
	}
	c := NewTelegram(configMap, testAppConfig(), testDeps)
	c.url = s.URL + "/%s"

	err := c.SendMessage(context.Background(), "test")
	assert.Nil(err)
}
